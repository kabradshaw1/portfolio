-- Seed data for authdb. Run by go/k8s/jobs/auth-service-migrate.yml
-- after `migrate up` succeeds. Every operation is idempotent so the Job can
-- re-run on every deploy without creating duplicates.

-- Smoke-test user. The password is stored in GitHub Actions secret
-- SMOKE_GO_PASSWORD; this hash must match it (bcrypt cost 10, the same
-- cost the auth-service uses via bcrypt.DefaultCost).
--
-- To regenerate the hash after rotating the password:
--   python3 -c "import bcrypt; print(bcrypt.hashpw(b'<new-password>', bcrypt.gensalt(10)).decode())"
INSERT INTO users (email, password_hash, name)
SELECT 'smoke@kylebradshaw.dev', '$2b$10$fBBfS.Pgxgqw2mavsb4cAOcavTkBkeZqMiXnM7e1nl6vCAOQ036Pq', 'Smoke Test'
WHERE NOT EXISTS (SELECT 1 FROM users WHERE email = 'smoke@kylebradshaw.dev');
