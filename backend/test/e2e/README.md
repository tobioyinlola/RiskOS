# Backend end-to-end tests

The end-to-end suite proves the core journey from spec §16/§23 through the public API only:

tenant → API key → event → validation → normalization → risk evaluation → score → decision →
alert → case → resolution → audit log

It is written as soon as that journey exists (TODO(phase-4)). Until then there is nothing
meaningful to test end to end beyond what `test/integration` and `test/security` already cover.
