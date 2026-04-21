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
    ('Water Bottle 32oz', 'Insulated stainless steel, keeps cold 24hrs', 2499, 'Sports', '', 200),
    -- Electronics (8 new → 12 total)
    ('Laptop Pro 15"', '15.6" FHD display, 16GB RAM, 512GB NVMe SSD, 10hr battery', 84999, 'Electronics', '', 25),
    ('Tablet 10" WiFi', '10.1" IPS display, 64GB storage, stylus support', 34999, 'Electronics', '', 35),
    ('27" 4K Monitor', 'IPS panel, USB-C PD 65W, 99% sRGB, adjustable stand', 44999, 'Electronics', '', 20),
    ('Smartwatch Sport', 'GPS, heart rate, 7-day battery, 5ATM water resistant', 24999, 'Electronics', '', 45),
    ('Wireless Earbuds Pro', 'Active noise canceling, 8hr battery, wireless charging case', 14999, 'Electronics', '', 60),
    ('1080p Webcam', 'Autofocus, dual mics, privacy shutter, USB-A/C', 5999, 'Electronics', '', 75),
    ('USB-C Hub 7-in-1', 'HDMI 4K, 3x USB-A, SD/microSD, PD 100W passthrough', 4499, 'Electronics', '', 85),
    ('WiFi 6 Router', 'Dual-band AX3000, 4x Gigabit LAN, MU-MIMO, WPA3', 9999, 'Electronics', '', 30),
    -- Clothing (6 new → 10 total)
    ('Trail Running Shoes', 'Vibram outsole, waterproof membrane, 8mm drop', 12999, 'Clothing', '', 40),
    ('Denim Jacket', 'Classic fit, 100% cotton denim, button front', 7999, 'Clothing', '', 50),
    ('Performance Polo', 'Moisture-wicking stretch fabric, UPF 30', 3999, 'Clothing', '', 90),
    ('Hiking Boots Mid', 'Waterproof leather, ankle support, Vibram sole', 16999, 'Clothing', '', 30),
    ('Athletic Shorts 7"', 'Quick-dry, zippered pocket, reflective trim', 2999, 'Clothing', '', 110),
    ('Zip-Up Hoodie', 'Heavyweight fleece, kangaroo pocket, ribbed cuffs', 5499, 'Clothing', '', 65),
    -- Home (8 new → 12 total)
    ('Robot Vacuum', 'LiDAR navigation, 2500Pa suction, self-emptying base', 39999, 'Home', '', 15),
    ('HEPA Air Purifier', 'Covers 400 sq ft, 3-stage filter, auto mode, quiet 25dB', 19999, 'Home', '', 25),
    ('Smart Thermostat', 'WiFi, learning schedule, energy reports, voice control', 14999, 'Home', '', 35),
    ('Chef Knife Set 5pc', 'German steel, full tang, ergonomic handles, block included', 12999, 'Home', '', 20),
    ('Stand Mixer 5qt', '10-speed, tilt-head, stainless bowl, 3 attachments included', 29999, 'Home', '', 18),
    ('French Press 34oz', 'Double-wall stainless steel, vacuum insulated, 4-level filter', 3499, 'Home', '', 80),
    ('Bath Towel Set 6pc', '100% Turkish cotton, 700 GSM, quick-dry, oeko-tex certified', 4999, 'Home', '', 55),
    ('Bamboo Cutting Board', 'End-grain, juice groove, rubber feet, 18x12"', 3999, 'Home', '', 70),
    -- Books (6 new → 10 total)
    ('Hands-On Machine Learning', 'Aurelien Geron — scikit-learn, Keras, TensorFlow', 5499, 'Books', '', 60),
    ('Cloud Native Patterns', 'Cornelia Davis — designing change-tolerant software', 4499, 'Books', '', 50),
    ('Database Internals', 'Alex Petrov — storage engines and distributed systems', 4999, 'Books', '', 45),
    ('Observability Engineering', 'Majors, Fong-Jones, Miranda — modern monitoring', 4499, 'Books', '', 55),
    ('Kubernetes in Action', 'Marko Luksa — hands-on container orchestration', 4999, 'Books', '', 40),
    ('Computer Networking', 'Kurose & Ross — top-down approach, 8th edition', 9999, 'Books', '', 35),
    -- Sports (7 new → 11 total)
    ('GPS Running Watch', 'Optical HR, pace alerts, 14-day battery, 50m water resist', 19999, 'Sports', '', 25),
    ('High-Density Foam Roller', '18" textured EVA, ideal for IT band and back', 2499, 'Sports', '', 90),
    ('Doorway Pull-Up Bar', 'No screws, fits 26-36" frames, 300lb capacity, padded grips', 3499, 'Sports', '', 60),
    ('Speed Jump Rope', 'Ball-bearing handles, adjustable steel cable, 10ft', 1499, 'Sports', '', 140),
    ('Gym Duffel Bag', '40L, shoe compartment, wet pocket, padded strap', 4499, 'Sports', '', 50),
    ('Cycling Gloves', 'Gel-padded palm, breathable mesh, touchscreen fingertips', 2499, 'Sports', '', 75),
    ('Hiking Backpack 40L', 'Rain cover, hydration compatible, hip belt, ventilated back', 8999, 'Sports', '', 30)
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
