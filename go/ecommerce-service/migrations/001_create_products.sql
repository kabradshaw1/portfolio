CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE IF NOT EXISTS products (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    price INTEGER NOT NULL,
    category VARCHAR(100) NOT NULL,
    image_url VARCHAR(500),
    stock INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_products_category ON products (category);
CREATE INDEX idx_products_price ON products (price);

INSERT INTO products (name, description, price, category, stock) VALUES
('Wireless Bluetooth Headphones', 'Noise-canceling over-ear headphones with 30hr battery', 7999, 'Electronics', 50),
('USB-C Fast Charger', 'GaN 65W charger with 3 ports', 3499, 'Electronics', 100),
('Mechanical Keyboard', 'RGB backlit with Cherry MX switches', 12999, 'Electronics', 30),
('Portable SSD 1TB', 'NVMe external drive, USB-C, 1050MB/s', 8999, 'Electronics', 40),
('Classic Cotton T-Shirt', 'Heavyweight premium cotton, unisex fit', 2499, 'Clothing', 200),
('Slim Fit Chinos', 'Stretch cotton blend, multiple colors available', 4999, 'Clothing', 80),
('Lightweight Rain Jacket', 'Packable waterproof shell with sealed seams', 6999, 'Clothing', 60),
('Merino Wool Beanie', 'Breathable and temperature regulating', 1999, 'Clothing', 150),
('Pour-Over Coffee Maker', 'Borosilicate glass with stainless steel filter', 3999, 'Home', 70),
('Cast Iron Skillet 12"', 'Pre-seasoned, oven safe to 500F', 4499, 'Home', 45),
('LED Desk Lamp', 'Adjustable brightness and color temperature', 5999, 'Home', 55),
('Ceramic Planter Set', 'Set of 3, drainage holes included', 2999, 'Home', 90),
('The Go Programming Language', 'Donovan & Kernighan — comprehensive Go guide', 3499, 'Books', 120),
('Designing Data-Intensive Applications', 'Martin Kleppmann — distributed systems bible', 3999, 'Books', 100),
('Clean Architecture', 'Robert C. Martin — software design principles', 2999, 'Books', 80),
('System Design Interview', 'Alex Xu — practical system design guide', 3499, 'Books', 95),
('Yoga Mat 6mm', 'Non-slip TPE material with carrying strap', 2999, 'Sports', 110),
('Adjustable Dumbbells', 'Quick-change weight from 5-52.5 lbs per hand', 29999, 'Sports', 20),
('Resistance Band Set', '5 bands with handles, door anchor, and bag', 1999, 'Sports', 130),
('Water Bottle 32oz', 'Insulated stainless steel, keeps cold 24hrs', 2499, 'Sports', 200);
