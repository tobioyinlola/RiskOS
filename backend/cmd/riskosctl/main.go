// Command riskosctl is the RiskOS operator tool. It performs operations that have no customer
// API: creating tenants, issuing a tenant's first keys, and verifying audit chains. Every change
// is audited in the tenant's log with the operator's name.
//
// Usage:
//
//	riskosctl tenant create --operator NAME --name NAME --region REGION
//	riskosctl apikey create --operator NAME --tenant ten_… --env test|production --name NAME [--scopes a,b]
//	riskosctl audit verify --tenant ten_…
//	riskosctl user invite --operator NAME --tenant ten_… --email EMAIL [--role customer_admin]
//
// Secrets are printed to stdout exactly once; logs go to stderr.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/config"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/db"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
	"github.com/tobioyinlola/RiskOS/backend/internal/tenants"
)

const usage = `usage:
  riskosctl tenant create --operator NAME --name NAME --region REGION
  riskosctl apikey create --operator NAME --tenant ten_... --env test|production --name NAME [--scopes a,b]
  riskosctl audit verify --tenant ten_...
  riskosctl user invite --operator NAME --tenant ten_... --email EMAIL [--role customer_admin]`

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "riskosctl:", err)
		os.Exit(1)
	}
}

// errUsage is returned after printing usage to stderr.
var errUsage = errors.New("invalid arguments")

func usageError() error {
	fmt.Fprintln(os.Stderr, usage)
	return errUsage
}

func run(args []string, out io.Writer) error {
	if len(args) < 2 {
		return usageError()
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return err
	}

	switch args[0] + " " + args[1] {
	case "tenant create":
		return tenantCreate(ctx, cfg, args[2:], out)
	case "apikey create":
		return apikeyCreate(ctx, cfg, args[2:], out)
	case "audit verify":
		return auditVerify(ctx, cfg, args[2:], out)
	case "user invite":
		return userInvite(ctx, cfg, args[2:], out)
	default:
		return usageError()
	}
}

// app wires the modules against the application role, exactly as the API does, so operator
// actions are subject to the same row-level security and auditing.
type app struct {
	tenants *tenants.Module
	auth    *auth.Module
	close   func()
}

func newApp(ctx context.Context, cfg config.Config) (*app, error) {
	if err := cfg.Auth.Validate(); err != nil {
		return nil, err
	}
	pool, err := db.NewPool(ctx, cfg.Postgres)
	if err != nil {
		return nil, err
	}
	clk := clock.System{}
	runner := db.NewTxRunner(pool)
	auditModule := audit.New(runner, clk)
	return &app{
		tenants: tenants.New(runner, auditModule.Recorder()),
		auth: auth.New(pool, runner, auditModule.Recorder(), clk, auth.Config{
			Pepper: []byte(cfg.Auth.APIKeyPepper), CacheTTL: 0,
			Cookie: auth.CookieConfig{ConsoleURL: cfg.Console.URL},
		}),
		close: pool.Close,
	}, nil
}

func operatorActor(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 100 {
		return "", errors.New("--operator is required (1-100 characters): who is running this command")
	}
	return "riskosctl:" + name, nil
}

func tenantCreate(ctx context.Context, cfg config.Config, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("tenant create", flag.ContinueOnError)
	operator := fs.String("operator", "", "who is running this command (recorded in the audit log)")
	name := fs.String("name", "", "tenant (organization) name")
	region := fs.String("region", "", "regional cell the tenant is pinned to, e.g. eu-west-1")
	if err := fs.Parse(args); err != nil {
		return err
	}
	actor, err := operatorActor(*operator)
	if err != nil {
		return err
	}
	a, err := newApp(ctx, cfg)
	if err != nil {
		return err
	}
	defer a.close()

	var key auth.CreatedKey
	t, err := a.tenants.Create(ctx, tenants.CreateParams{Name: *name, Region: *region, Actor: actor},
		func(ctx context.Context, _ tenants.Tenant) error {
			key, err = a.auth.CreateKey(ctx, auth.CreateKeyInput{
				Name:   "bootstrap (created by " + actor + ")",
				Scopes: authz.CustomerPermissions(),
			})
			return err
		})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, `Tenant created.
  tenant:       %s
  region:       %s
  test env:     %s
  production:   %s

Test API key (customer_admin scopes). Shown once; store it in a secrets manager now:
  %s

Create a production key with: riskosctl apikey create --env production ...
`, ids.Format(ids.PrefixTenant, t.ID), t.Region,
		ids.Format(ids.PrefixEnvironment, t.TestEnvID), ids.Format(ids.PrefixEnvironment, t.ProductionEnv), key.Secret)
	return err
}

func apikeyCreate(ctx context.Context, cfg config.Config, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("apikey create", flag.ContinueOnError)
	operator := fs.String("operator", "", "who is running this command (recorded in the audit log)")
	tenant := fs.String("tenant", "", "tenant ID (ten_…)")
	env := fs.String("env", "", "test or production")
	name := fs.String("name", "", "key name")
	scopes := fs.String("scopes", strings.Join(authz.CustomerPermissions(), ","), "comma-separated scopes")
	if err := fs.Parse(args); err != nil {
		return err
	}
	actor, err := operatorActor(*operator)
	if err != nil {
		return err
	}
	tenantID, err := ids.Parse(ids.PrefixTenant, *tenant)
	if err != nil {
		return fmt.Errorf("--tenant: %w", err)
	}
	a, err := newApp(ctx, cfg)
	if err != nil {
		return err
	}
	defer a.close()

	scoped, err := a.tenants.OperatorScope(ctx, tenantID, tenancy.EnvironmentKind(*env), actor)
	if err != nil {
		return err
	}
	key, err := a.auth.CreateKey(scoped, auth.CreateKeyInput{Name: *name, Scopes: strings.Split(*scopes, ",")})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "API key %s created in %s. Shown once; store it in a secrets manager now:\n  %s\n",
		ids.Format(ids.PrefixAPIKey, key.Key.ID), *env, key.Secret)
	return err
}

// userInvite invites a person to a tenant's console, usually its first administrator. The
// link is printed once; the person sets a password and enrolls a second factor.
func userInvite(ctx context.Context, cfg config.Config, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("user invite", flag.ContinueOnError)
	operator := fs.String("operator", "", "who is running this command (recorded in the audit log)")
	tenant := fs.String("tenant", "", "tenant ID (ten_…)")
	email := fs.String("email", "", "the person's email address")
	role := fs.String("role", string(authz.RoleCustomerAdmin), "customer_admin, developer, risk_manager or analyst")
	if err := fs.Parse(args); err != nil {
		return err
	}
	actor, err := operatorActor(*operator)
	if err != nil {
		return err
	}
	tenantID, err := ids.Parse(ids.PrefixTenant, *tenant)
	if err != nil {
		return fmt.Errorf("--tenant: %w", err)
	}
	a, err := newApp(ctx, cfg)
	if err != nil {
		return err
	}
	defer a.close()
	// Users belong to the tenant; the operator scope needs an environment only to exist.
	scoped, err := a.tenants.OperatorScope(ctx, tenantID, tenancy.EnvironmentTest, actor)
	if err != nil {
		return err
	}
	link, err := a.auth.Invite(scoped, *email, authz.Role(*role))
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Invite created for %s (%s). Valid for 72 hours, usable once. Share it directly:\n  %s\n",
		strings.ToLower(strings.TrimSpace(*email)), *role, link)
	return err
}

func auditVerify(ctx context.Context, cfg config.Config, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("audit verify", flag.ContinueOnError)
	tenant := fs.String("tenant", "", "tenant ID (ten_…)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	tenantID, err := ids.Parse(ids.PrefixTenant, *tenant)
	if err != nil {
		return fmt.Errorf("--tenant: %w", err)
	}
	// Verification must see every environment's entries, so it reads as the schema owner.
	if cfg.Postgres.MigrationURL == "" {
		return errors.New("RISKOS_POSTGRES_MIGRATION_URL is required to verify the audit chain")
	}
	ownerCfg := cfg.Postgres
	ownerCfg.URL = cfg.Postgres.MigrationURL
	pool, err := db.NewPool(ctx, ownerCfg)
	if err != nil {
		return err
	}
	defer pool.Close()

	res, err := audit.Verify(ctx, pool, tenantID)
	if err != nil {
		return err
	}
	if !res.Valid {
		_, _ = fmt.Fprintf(out, "INVALID: %d entries verified; first problem at sequence %d: %s\n", res.Entries, res.FirstInvalidSeq, res.Reason)
		return errors.New("audit chain verification failed")
	}
	_, err = fmt.Fprintf(out, "OK: %d entries, chain intact up to sequence %d.\n", res.Entries, res.HeadSeq)
	return err
}
