# Backend end-to-end tests

The core journey from spec §16/§23 is proven through the public API only:

tenant → API key → event → validation → normalization → risk evaluation → score → decision →
alert → case → resolution → audit log

It is `TestEndToEndAlertToResolution` in [`test/integration/alerts_test.go`](../integration/alerts_test.go),
run against real PostgreSQL and Redis by `make test-integration`. The browser journeys through
the console are in `frontend/tests/e2e` (`make test-e2e`).

This directory has no tests of its own: a separate suite would repeat that test. It is the place
for journeys that need the API and worker as separate processes behind a gateway, which belong
with the deployment work (TODO(phase-8)).
