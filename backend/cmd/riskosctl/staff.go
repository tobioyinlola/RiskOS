package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/tobioyinlola/RiskOS/backend/internal/admin"
	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/config"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/db"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/tenants"
)

// staffApp wires the admin realm for operator commands. Staff accounts exist only through these
// commands; there is no API to create one.
func staffApp(ctx context.Context, cfg config.Config) (*admin.Module, func(), error) {
	pool, err := db.NewPool(ctx, cfg.Postgres)
	if err != nil {
		return nil, nil, err
	}
	clk := clock.System{}
	runner := db.NewTxRunner(pool)
	rec := audit.New(runner, clk).Recorder()
	m, err := admin.New(pool, tenants.New(runner, rec), runner, rec, clk, admin.Config{Secret: []byte(cfg.Console.Secret), Origins: []string{cfg.Console.URL}})
	if err != nil {
		pool.Close()
		return nil, nil, err
	}
	return m, pool.Close, nil
}

func operatorName(name string) (string, error) {
	if _, err := operatorActor(name); err != nil {
		return "", err
	}
	return strings.TrimSpace(name), nil
}

func staffCommand(ctx context.Context, cfg config.Config, verb string, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("staff "+verb, flag.ContinueOnError)
	operator := fs.String("operator", "", "who is running this command (recorded in the staff audit log)")
	email := fs.String("email", "", "the staff member's email address")
	name := fs.String("name", "", "the staff member's name (create)")
	role := fs.String("role", string(authz.RoleOperationsAdmin), "operations_admin or security_admin (create)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	m, closeFn, err := staffApp(ctx, cfg)
	if err != nil {
		return err
	}
	defer closeFn()

	if verb == "list" {
		staff, err := m.ListStaff(ctx)
		if err != nil {
			return err
		}
		tw := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tEMAIL\tNAME\tROLE\tSTATUS\tCREATED BY")
		for _, s := range staff {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", ids.Format(ids.PrefixStaff, s.ID), s.Email, s.Name, s.Role, s.Status, s.CreatedBy)
		}
		return tw.Flush()
	}

	op, err := operatorName(*operator)
	if err != nil {
		return err
	}
	if strings.TrimSpace(*email) == "" {
		return errors.New("--email is required")
	}
	switch verb {
	case "create":
		token, err := m.CreateStaff(ctx, *email, *name, authz.Role(*role), op)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "Staff member %s (%s) created. Send them this link to register their passkey; it works once, within 24 hours:\n  %s\n",
			strings.ToLower(strings.TrimSpace(*email)), *role, admin.EnrollURL(cfg.Console.URL, token))
		return err
	case "enroll":
		token, err := m.ReissueEnrollment(ctx, *email, op)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "New passkey link for %s; it works once, within 24 hours:\n  %s\n",
			strings.ToLower(strings.TrimSpace(*email)), admin.EnrollURL(cfg.Console.URL, token))
		return err
	case "unbind-sso":
		if err := m.UnbindSSO(ctx, *email, op); err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "Single sign-on identity of %s removed and their sessions ended. Their next single sign-on binds afresh.\n",
			strings.ToLower(strings.TrimSpace(*email)))
		return err
	case "disable", "enable":
		if err := m.SetStaffStatus(ctx, *email, verb == "enable", op); err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "Staff member %s %sd.\n", strings.ToLower(strings.TrimSpace(*email)), verb)
		return err
	default:
		return usageError()
	}
}
