-- Seed data for ecommercedb. Run by go/k8s/jobs/ecommerce-service-migrate.yml
-- after `migrate up` succeeds. Every operation is idempotent so the Job can
-- re-run on every deploy without creating duplicates.

-- Product catalog (20 rows). image_url is '' (not NULL) because the Go
-- model.Product.ImageURL is a plain string and would fail to scan NULL.
INSERT INTO products (name, description, price, category, image_url, stock)
SELECT * FROM (VALUES
    ('Wireless Bluetooth Headphones', 'Noise-canceling over-ear headphones with 30hr battery', 7999, 'Electronics', '', 50),
    ('USB-C Fast Charger', 'GaN 65W charger with 3 ports', 3499, 'Electronics', '', 100),
    ('Mechanical Keyboard', 'RGB backlit with Cherry MX switches', 12999, 'Electronics', '', 30),
    ('Portable SSD 1TB', 'NVMe external drive, USB-C, 1050MB/s', 8999, 'Electronics', '', 40),
    ('Classic Cotton T-Shirt', 'Heavyweight premium cotton, unisex fit', 2499, 'Clothing', '', 200),
    ('Slim Fit Chinos', 'Stretch cotton blend, multiple colors available', 4999, 'Clothing', '', 80),
    ('Lightweight Rain Jacket', 'Packable waterproof shell with sealed seams', 6999, 'Clothing', '', 60),
    ('Merino Wool Beanie', 'Breathable and temperature regulating', 1999, 'Clothing', '', 150),
    ('Pour-Over Coffee Maker', 'Borosilicate glass with stainless steel filter', 3999, 'Home', '', 70),
    ('Cast Iron Skillet 12"', 'Pre-seasoned, oven safe to 500F', 4499, 'Home', '', 45),
    ('LED Desk Lamp', 'Adjustable brightness and color temperature', 5999, 'Home', '', 55),
    ('Ceramic Planter Set', 'Set of 3, drainage holes included', 2999, 'Home', '', 90),
    ('The Go Programming Language', 'Donovan & Kernighan — comprehensive Go guide', 3499, 'Books', '', 120),
    ('Designing Data-Intensive Applications', 'Martin Kleppmann — distributed systems bible', 3999, 'Books', '', 100),
    ('Clean Architecture', 'Robert C. Martin — software design principles', 2999, 'Books', '', 80),
    ('System Design Interview', 'Alex Xu — practical system design guide', 3499, 'Books', '', 95),
    ('Yoga Mat 6mm', 'Non-slip TPE material with carrying strap', 2999, 'Sports', '', 110),
    ('Adjustable Dumbbells', 'Quick-change weight from 5-52.5 lbs per hand', 29999, 'Sports', '', 20),
    ('Resistance Band Set', '5 bands with handles, door anchor, and bag', 1999, 'Sports', '', 130),
    ('Water Bottle 32oz', 'Insulated stainless steel, keeps cold 24hrs', 2499, 'Sports', '', 200)
) AS v(name, description, price, category, image_url, stock)
WHERE NOT EXISTS (SELECT 1 FROM products);

-- Smoke-test user. The password is stored in GitHub Actions secret
-- SMOKE_GO_PASSWORD; this hash must match it (bcrypt cost 10, the same
-- cost the auth-service uses via bcrypt.DefaultCost).
--
-- To regenerate the hash after rotating the password:
--   python3 -c "import bcrypt; print(bcrypt.hashpw(b'<new-password>', bcrypt.gensalt(10)).decode())"
INSERT INTO users (email, password_hash, name)
SELECT 'smoke@kylebradshaw.dev', '$2b$10$fBBfS.Pgxgqw2mavsb4cAOcavTkBkeZqMiXnM7e1nl6vCAOQ036Pq', 'Smoke Test'
WHERE NOT EXISTS (SELECT 1 FROM users WHERE email = 'smoke@kylebradshaw.dev');

-- Smoke-test product with effectively unlimited stock so automated tests
-- never deplete inventory or affect the demo catalog.
INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Smoke Test Widget', 'Reserved for automated smoke tests', 100, 'Electronics', '', 999999
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Smoke Test Widget');
