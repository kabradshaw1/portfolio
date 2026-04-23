-- Seed data for productdb. Run by go/k8s/jobs/product-service-migrate.yml
-- after `migrate up` succeeds. Every operation is idempotent so the Job can
-- re-run on every deploy without creating duplicates.
-- Each INSERT is guarded by a per-product name check (not a table-wide check),
-- so new products can be added without being blocked by existing rows.

-- Electronics (8 items)
INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Wireless Bluetooth Headphones', 'Noise-canceling over-ear headphones with 30hr battery', 7999, 'Electronics', '', 50
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Wireless Bluetooth Headphones');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'USB-C Fast Charger', 'GaN 65W charger with 3 ports', 3499, 'Electronics', '', 100
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'USB-C Fast Charger');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Mechanical Keyboard', 'RGB backlit with Cherry MX switches', 12999, 'Electronics', '', 30
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Mechanical Keyboard');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Portable SSD 1TB', 'NVMe external drive, USB-C, 1050MB/s', 8999, 'Electronics', '', 40
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Portable SSD 1TB');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT '4K Webcam', 'Ultra HD 4K streaming webcam with autofocus and noise-canceling mic', 9999, 'Electronics', '', 60
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = '4K Webcam');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT '27" Monitor', 'IPS panel, 144Hz, 1ms response time, HDR400', 34999, 'Electronics', '', 25
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = '27" Monitor');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Smartwatch', 'GPS, heart rate, SpO2, 7-day battery life', 19999, 'Electronics', '', 45
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Smartwatch');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Wireless Earbuds', 'Active noise canceling, 28hr total battery with case', 12999, 'Electronics', '', 75
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Wireless Earbuds');

-- Clothing (8 items)
INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Classic Cotton T-Shirt', 'Heavyweight premium cotton, unisex fit', 2499, 'Clothing', '', 200
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Classic Cotton T-Shirt');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Slim Fit Chinos', 'Stretch cotton blend, multiple colors available', 4999, 'Clothing', '', 80
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Slim Fit Chinos');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Lightweight Rain Jacket', 'Packable waterproof shell with sealed seams', 6999, 'Clothing', '', 60
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Lightweight Rain Jacket');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Merino Wool Beanie', 'Breathable and temperature regulating', 1999, 'Clothing', '', 150
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Merino Wool Beanie');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Running Shoes', 'Lightweight mesh upper with responsive foam midsole', 8999, 'Clothing', '', 70
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Running Shoes');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Denim Jacket', 'Classic fit, 100% cotton denim with button closure', 5999, 'Clothing', '', 55
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Denim Jacket');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Performance Polo', 'Moisture-wicking fabric, UPF 30+ sun protection', 3499, 'Clothing', '', 120
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Performance Polo');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Zip-Up Hoodie', 'French terry cotton blend, kangaroo pocket', 4499, 'Clothing', '', 90
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Zip-Up Hoodie');

-- Home (8 items)
INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Pour-Over Coffee Maker', 'Borosilicate glass with stainless steel filter', 3999, 'Home', '', 70
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Pour-Over Coffee Maker');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Cast Iron Skillet 12"', 'Pre-seasoned, oven safe to 500F', 4499, 'Home', '', 45
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Cast Iron Skillet 12"');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'LED Desk Lamp', 'Adjustable brightness and color temperature', 5999, 'Home', '', 55
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'LED Desk Lamp');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Ceramic Planter Set', 'Set of 3, drainage holes included', 2999, 'Home', '', 90
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Ceramic Planter Set');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Robot Vacuum', 'LiDAR mapping, auto-empty base, 180min battery', 29999, 'Home', '', 20
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Robot Vacuum');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Air Purifier', 'True HEPA filter, covers 500 sq ft, quiet operation', 14999, 'Home', '', 35
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Air Purifier');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Smart Thermostat', 'Wi-Fi enabled, energy usage reports, compatible with Alexa', 12999, 'Home', '', 40
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Smart Thermostat');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Chef Knife Set', '8-piece high-carbon stainless steel with wooden block', 7999, 'Home', '', 30
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Chef Knife Set');

-- Books (8 items)
INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'The Go Programming Language', 'Donovan & Kernighan — comprehensive Go guide', 3499, 'Books', '', 120
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'The Go Programming Language');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Designing Data-Intensive Applications', 'Martin Kleppmann — distributed systems bible', 3999, 'Books', '', 100
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Designing Data-Intensive Applications');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Clean Architecture', 'Robert C. Martin — software design principles', 2999, 'Books', '', 80
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Clean Architecture');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'System Design Interview', 'Alex Xu — practical system design guide', 3499, 'Books', '', 95
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'System Design Interview');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Database Internals', 'Alex Petrov — deep dive into storage engines and distributed systems', 4499, 'Books', '', 65
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Database Internals');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Observability Engineering', 'Charity Majors et al. — achieving production excellence', 3999, 'Books', '', 55
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Observability Engineering');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Kubernetes in Action', 'Marko Luksa — comprehensive Kubernetes guide', 4999, 'Books', '', 70
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Kubernetes in Action');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Networking and Kubernetes', 'James Strong & Vallery Lancey — container networking deep dive', 3999, 'Books', '', 50
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Networking and Kubernetes');

-- Sports (8 items)
INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Yoga Mat 6mm', 'Non-slip TPE material with carrying strap', 2999, 'Sports', '', 110
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Yoga Mat 6mm');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Adjustable Dumbbells', 'Quick-change weight from 5-52.5 lbs per hand', 29999, 'Sports', '', 20
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Adjustable Dumbbells');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Resistance Band Set', '5 bands with handles, door anchor, and bag', 1999, 'Sports', '', 130
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Resistance Band Set');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Water Bottle 32oz', 'Insulated stainless steel, keeps cold 24hrs', 2499, 'Sports', '', 200
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Water Bottle 32oz');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Running Watch', 'GPS, heart rate, pace alerts, 14-day battery', 24999, 'Sports', '', 30
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Running Watch');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Foam Roller', 'High-density EVA foam, textured surface for deep tissue', 2499, 'Sports', '', 85
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Foam Roller');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Pull-Up Bar', 'Doorframe mounted, no screws, 300lb capacity', 3499, 'Sports', '', 60
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Pull-Up Bar');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Hiking Backpack 40L', 'Waterproof, hip belt, hydration bladder compatible', 8999, 'Sports', '', 45
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Hiking Backpack 40L');

-- Smoke-test product with effectively unlimited stock so automated tests
-- never deplete inventory or affect the demo catalog.
INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Smoke Test Widget', 'Reserved for automated smoke tests', 100, 'Electronics', '', 999999
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Smoke Test Widget');
