package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/riverqueue/river/rivertype"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/appmetrics"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/db"
)

type riverWorkers = river.Workers

func addRiverWorker[T Args](w *riverWorkers, handler func(context.Context, T) error) error {
	return river.AddWorkerSafely(w, river.WorkFunc(func(ctx context.Context, job *river.Job[T]) error {
		return handler(ctx, job.Args)
	}))
}

// Queue is the River-backed job queue. It implements Enqueuer and, when handlers are
// registered, executes jobs.
type Queue struct {
	client *river.Client[pgx.Tx]
}

var _ Enqueuer = (*Queue)(nil)

// QueueConfig configures a Queue.
type QueueConfig struct {
	// Concurrency is the maximum number of jobs executed at once.
	Concurrency int
	// JobTimeout bounds each job's context.
	JobTimeout time.Duration
	// Queues are additional queues (see Queued) and their maximum concurrent jobs.
	Queues map[string]int
	// Metrics records job durations and queue depth; nil records nothing.
	Metrics *appmetrics.Metrics
}

// jobMetrics times each job and records its outcome by kind.
type jobMetrics struct {
	river.WorkerMiddlewareDefaults
	m *appmetrics.Metrics
}

func (j *jobMetrics) Work(ctx context.Context, job *rivertype.JobRow, doInner func(context.Context) error) error {
	start := time.Now()
	err := doInner(ctx)
	outcome := appmetrics.JobSucceeded
	var snooze *river.JobSnoozeError
	switch {
	case errors.As(err, &snooze):
		outcome = appmetrics.JobSnoozed
	case err != nil:
		outcome = appmetrics.JobFailed
	}
	j.m.JobRan(ctx, job.Kind, outcome, time.Since(start))
	return err
}

// queueDepth counts jobs not yet finished, by queue and state. The state filter keeps it on the
// partial index River keeps for fetching, so it stays cheap as finished jobs accumulate.
func queueDepth(pool *pgxpool.Pool) func(context.Context) ([]appmetrics.QueueDepth, error) {
	return func(ctx context.Context) ([]appmetrics.QueueDepth, error) {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		rows, err := pool.Query(ctx, `
			SELECT queue, state::text, count(*)
			FROM river_job
			WHERE state IN ('available', 'retryable', 'scheduled', 'running')
			GROUP BY queue, state`)
		if err != nil {
			return nil, fmt.Errorf("count queued jobs: %w", err)
		}
		defer rows.Close()
		var out []appmetrics.QueueDepth
		for rows.Next() {
			var d appmetrics.QueueDepth
			if err := rows.Scan(&d.Queue, &d.State, &d.Count); err != nil {
				return nil, fmt.Errorf("scan queued jobs: %w", err)
			}
			out = append(out, d)
		}
		return out, rows.Err()
	}
}

// Snooze returns an error that makes the queue run the job again after d, without counting an
// attempt. For handlers that manage their own retry schedule.
func Snooze(d time.Duration) error { return river.JobSnooze(d) }

// NewQueue builds a queue on pool that executes the handlers in registry.
func NewQueue(pool *pgxpool.Pool, registry *Registry, cfg QueueConfig, logger *slog.Logger) (*Queue, error) {
	workers := river.NewWorkers()
	for _, register := range registry.registrations {
		if err := register(workers); err != nil {
			return nil, fmt.Errorf("register job handler: %w", err)
		}
	}
	periodic := make([]*river.PeriodicJob, 0, len(registry.periodic))
	for _, p := range registry.periodic {
		args := p.args
		periodic = append(periodic, river.NewPeriodicJob(
			river.PeriodicInterval(p.interval),
			func() (river.JobArgs, *river.InsertOpts) { return args, nil },
			&river.PeriodicJobOpts{RunOnStart: true},
		))
	}
	queues := map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: cfg.Concurrency}}
	for name, n := range cfg.Queues {
		queues[name] = river.QueueConfig{MaxWorkers: n}
	}
	// Jobs held by a worker that died (SIGKILL, lost node) are retried once they have been
	// running longer than any job may run, instead of after River's default hour.
	var rescueAfter time.Duration
	if cfg.JobTimeout > 0 {
		rescueAfter = cfg.JobTimeout + time.Minute
	}
	var middleware []rivertype.Middleware
	if cfg.Metrics != nil {
		middleware = append(middleware, &jobMetrics{m: cfg.Metrics})
		if err := cfg.Metrics.ObserveQueueDepth(queueDepth(pool)); err != nil {
			return nil, err
		}
	}
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Logger:               logger,
		JobTimeout:           cfg.JobTimeout,
		Middleware:           middleware,
		RescueStuckJobsAfter: rescueAfter,
		Workers:              workers,
		PeriodicJobs:         periodic,
		Queues:               queues,
	})
	if err != nil {
		return nil, fmt.Errorf("create river client: %w", err)
	}
	return &Queue{client: client}, nil
}

// NewInserter returns a queue that only inserts jobs (the API process): it runs no workers.
func NewInserter(pool *pgxpool.Pool, logger *slog.Logger) (*Queue, error) {
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Logger: logger})
	if err != nil {
		return nil, fmt.Errorf("create river client: %w", err)
	}
	return &Queue{client: client}, nil
}

// Enqueue inserts a job. Inside a tenant transaction (txn.Runner) the job joins it, so it is
// created only if the transaction commits: the queue then acts as a transactional outbox.
func (q *Queue) Enqueue(ctx context.Context, args Args) error {
	var opts *river.InsertOpts
	if qd, ok := args.(Queued); ok {
		opts = &river.InsertOpts{Queue: qd.Queue()}
	}
	var err error
	if tx, txErr := db.TxFrom(ctx); txErr == nil {
		_, err = q.client.InsertTx(ctx, tx, args, opts)
	} else {
		_, err = q.client.Insert(ctx, args, opts)
	}
	if err != nil {
		return fmt.Errorf("enqueue %s job: %w", args.Kind(), err)
	}
	return nil
}

// Start begins fetching and executing jobs. It returns once workers are running.
func (q *Queue) Start(ctx context.Context) error {
	if err := q.client.Start(ctx); err != nil {
		return fmt.Errorf("start job queue: %w", err)
	}
	return nil
}

// Stop stops fetching new jobs and waits for running jobs to finish, bounded by ctx.
func (q *Queue) Stop(ctx context.Context) error {
	if err := q.client.Stop(ctx); err != nil {
		return fmt.Errorf("stop job queue: %w", err)
	}
	return nil
}

// Migrate applies the queue's own schema migrations and grants appRole (the application role
// the api and worker connect as) the access River needs to its tables. It is run by cmd/migrate,
// as the schema owner, after the application migrations.
func Migrate(ctx context.Context, pool *pgxpool.Pool, appRole string, logger *slog.Logger) error {
	m, err := rivermigrate.New(riverpgxv5.New(pool), &rivermigrate.Config{Logger: logger})
	if err != nil {
		return fmt.Errorf("create river migrator: %w", err)
	}
	res, err := m.Migrate(ctx, rivermigrate.DirectionUp, nil)
	if err != nil {
		return fmt.Errorf("migrate river schema: %w", err)
	}
	for _, v := range res.Versions {
		logger.InfoContext(ctx, "applied job queue migration", slog.Int("version", v.Version))
	}
	// Grants are idempotent and re-applied on every run, so they also cover tables added by River
	// upgrades. A DO block cannot take parameters, so the role name travels in a
	// transaction-local setting, and every identifier is quoted with format(%I).
	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('riskos.app_role', $1, true)`, appRole); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			DO $grant$
			DECLARE
			  role_name text := current_setting('riskos.app_role');
			  obj text;
			BEGIN
			  FOR obj IN SELECT tablename FROM pg_tables
			             WHERE schemaname = 'public' AND tablename LIKE 'river\_%' LOOP
			    EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON public.%I TO %I', obj, role_name);
			  END LOOP;
			  FOR obj IN SELECT sequence_name FROM information_schema.sequences
			             WHERE sequence_schema = 'public' AND sequence_name LIKE 'river\_%' LOOP
			    EXECUTE format('GRANT USAGE, SELECT ON SEQUENCE public.%I TO %I', obj, role_name);
			  END LOOP;
			END
			$grant$`)
		return err
	})
	if err != nil {
		return fmt.Errorf("grant job queue access: %w", err)
	}
	return nil
}
