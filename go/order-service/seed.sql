-- Seed data for orderdb. Run by go/k8s/jobs/order-service-migrate.yml
-- after `migrate up` succeeds. Every operation is idempotent so the Job can
-- re-run on every deploy without creating duplicates.
--
-- After service decomposition, products live in productdb (product-service)
-- and users live in authdb (auth-service). This file seeds only order-related
-- data in orderdb.

-- (no order-specific seed data required at this time)
