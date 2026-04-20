# Unified AI Assistant Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enhance the Shopping Assistant drawer with rich tool result rendering, expand the product catalog to ~55 items, and pre-seed product PDFs for RAG document search.

**Architecture:** The Go ai-service already has all 12 tools (9 ecommerce + 3 RAG) registered and returns typed `display` payloads with a `kind` discriminator. This plan adds frontend components that switch on `display.kind` to render rich results instead of raw JSON, expands the seed data, and creates product PDFs for the RAG system.

**Tech Stack:** Next.js, TypeScript, shadcn/ui, Tailwind CSS, Playwright (E2E), Bash (seed script)

---

### Task 1: Expand Product Catalog in seed.sql

**Files:**
- Modify: `go/ecommerce-service/seed.sql`

- [ ] **Step 1: Read the current seed.sql to understand the pattern**

The current file has 20 products inserted via a single `INSERT ... SELECT * FROM (VALUES ...) WHERE NOT EXISTS`. The same idempotent pattern must be preserved — the `WHERE NOT EXISTS (SELECT 1 FROM products)` guard means ALL products are inserted as one batch or none.

- [ ] **Step 2: Add ~35 new products to the VALUES list**

Add the following products to the existing `INSERT INTO products` statement, inside the `VALUES` block, after the last existing row (`'Water Bottle 32oz'`):

```sql
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
```

These go inside the existing `VALUES (...)` block, before the closing `) AS v(...)` line. Separate from the previous last row with a comma.

- [ ] **Step 3: Run Go preflight to make sure syntax is valid**

Run: `make preflight-go`
Expected: PASS (seed.sql isn't executed in tests, but the Go linter/tests should still pass)

- [ ] **Step 4: Commit**

```bash
git add go/ecommerce-service/seed.sql
git commit -m "feat(ecommerce): expand product catalog from 20 to 55 items

Add 35 new products across all 5 categories with detailed descriptions
to make the shopping assistant more useful for demo purposes."
```

---

### Task 2: Create Product PDFs

**Files:**
- Create: `docs/product-catalog/electronics-buying-guide.pdf`
- Create: `docs/product-catalog/home-kitchen-guide.pdf`
- Create: `docs/product-catalog/fitness-equipment-guide.pdf`
- Create: `docs/product-catalog/laptop-pro-15-specs.pdf`
- Create: `docs/product-catalog/27-4k-monitor-specs.pdf`
- Create: `docs/product-catalog/smartwatch-sport-specs.pdf`
- Create: `docs/product-catalog/robot-vacuum-specs.pdf`
- Create: `docs/product-catalog/stand-mixer-5qt-specs.pdf`

PDFs will be generated from Markdown source files using a Python script with the `fpdf2` library (lightweight, no system dependencies). The Markdown sources are kept alongside the PDFs for maintainability.

- [ ] **Step 1: Create the docs/product-catalog/ directory**

```bash
mkdir -p docs/product-catalog
```

- [ ] **Step 2: Create Markdown source for the electronics buying guide**

Create `docs/product-catalog/electronics-buying-guide.md`:

```markdown
# Electronics Buying Guide

## Laptops

When choosing a laptop, consider these key factors:

### Display
- **FHD (1920x1080)**: Great for everyday tasks and coding. The Laptop Pro 15" offers a 15.6" FHD IPS panel with excellent color accuracy.
- **Higher resolutions**: Consider a 4K display if you do photo/video editing.

### Performance
- **RAM**: 16GB is the sweet spot for multitasking and development. The Laptop Pro 15" comes with 16GB standard.
- **Storage**: NVMe SSDs are 5-10x faster than SATA SSDs. The Laptop Pro 15" includes a 512GB NVMe drive with read speeds up to 3500MB/s.

### Battery Life
- The Laptop Pro 15" delivers up to 10 hours of real-world use with its 72Wh battery.
- For extended sessions, the USB-C Fast Charger (65W GaN) can charge from 0-50% in 30 minutes.

### Connectivity
- USB-C is essential. The USB-C Hub 7-in-1 adds HDMI 4K output, 3x USB-A, SD card slots, and 100W power delivery passthrough.

## Monitors

### Panel Technology
- **IPS**: Best color accuracy and viewing angles. The 27" 4K Monitor uses an IPS panel with 99% sRGB coverage.
- **VA**: Better contrast but narrower viewing angles.

### Resolution and Size
- **4K at 27"**: Pixel density of 163 PPI — crisp text and detailed images. The 27" 4K Monitor supports USB-C PD 65W, so one cable connects and charges your laptop.

### Ergonomics
- Adjustable stand (height, tilt, swivel, pivot) reduces neck strain. The 27" 4K Monitor includes a fully adjustable stand.

## Audio

### Over-Ear Headphones
- The Wireless Bluetooth Headphones offer active noise canceling with a 30-hour battery. Ideal for open offices and travel.
- 40mm custom drivers deliver balanced sound across all frequencies.

### Earbuds
- The Wireless Earbuds Pro feature ANC with transparency mode, 8-hour battery (32 hours with case), and IPX4 sweat resistance.
- Wireless charging case is compatible with any Qi charger.

## Networking

### WiFi Routers
- The WiFi 6 Router supports AX3000 speeds (up to 3Gbps combined) with MU-MIMO for multiple simultaneous connections.
- WPA3 encryption provides the latest security standard.
- 4x Gigabit LAN ports for wired devices that need maximum reliability.

## Webcams
- The 1080p Webcam features autofocus, dual noise-canceling microphones, and a physical privacy shutter.
- Works with USB-A and USB-C connections — no drivers needed.

## Accessories
- **Portable SSD 1TB**: NVMe speeds up to 1050MB/s over USB-C. Bus-powered, no external power needed.
- **Mechanical Keyboard**: Cherry MX switches rated for 100 million keystrokes. RGB backlighting with per-key customization.
```

- [ ] **Step 3: Create Markdown source for the home and kitchen guide**

Create `docs/product-catalog/home-kitchen-guide.md`:

```markdown
# Home & Kitchen Guide

## Kitchen Appliances

### Stand Mixer
The Stand Mixer 5qt is a kitchen workhorse with 10 speed settings and a tilt-head design for easy bowl access.

**Specifications:**
- Motor: 325-watt DC motor
- Bowl: 5-quart stainless steel with handle
- Speeds: 10 settings from stir to high
- Attachments included: flat beater, dough hook, wire whip
- Dimensions: 14" x 9" x 14" (H x W x D)
- Weight: 22 lbs
- Warranty: 2 years

**Compatible accessories (sold separately):** pasta roller, food grinder, spiralizer, ice cream maker, grain mill.

### Coffee Brewing
- **Pour-Over Coffee Maker**: Borosilicate glass carafe with reusable stainless steel mesh filter. No paper filters needed. Brews 4-6 cups.
- **French Press 34oz**: Double-wall vacuum insulated stainless steel keeps coffee hot for 2+ hours. 4-level filtration system eliminates sediment.

## Cookware

### Cast Iron
The Cast Iron Skillet 12" comes pre-seasoned and ready to use. Oven safe to 500°F, compatible with all cooktops including induction.

**Care instructions:**
1. Hand wash with hot water (minimal soap is fine)
2. Dry immediately and thoroughly
3. Apply a thin layer of cooking oil after each wash
4. Store in a dry place

### Knife Set
The Chef Knife Set 5pc features German high-carbon stainless steel blades with full-tang construction.

**Included knives:**
- 8" chef knife — all-purpose cutting and chopping
- 8" bread knife — serrated edge for crusty loaves
- 7" santoku — precision slicing and dicing
- 5" utility knife — medium tasks, trimming
- 3.5" paring knife — peeling and detail work
- Hardwood knife block

**Blade specs:** 58 HRC hardness, ice-tempered for edge retention, hand-honed to 15° angle per side.

### Cutting Boards
The Bamboo Cutting Board uses end-grain construction, which is gentler on knife edges than edge-grain. Features juice groove and rubber feet for stability. 18" x 12" x 1.5".

## Cleaning

### Robot Vacuum
The Robot Vacuum uses LiDAR navigation to map your home and clean in efficient straight lines rather than random bouncing.

**Specifications:**
- Suction: 2500Pa (adjustable in app)
- Runtime: Up to 150 minutes on hard floors
- Dustbin: 400ml onboard, 2.5L self-emptying base
- Navigation: LiDAR 360° scanning
- Mapping: Multi-floor maps, no-go zones, room-specific cleaning
- Noise: 55dB on standard mode
- Height: 3.5" — fits under most furniture
- Filter: HEPA, captures 99.97% of particles
- Connectivity: WiFi, app control, voice assistant compatible
- Warranty: 1 year

### Towels
Bath Towel Set 6pc (2 bath, 2 hand, 2 washcloths) made from 100% Turkish cotton at 700 GSM. OEKO-TEX certified free from harmful substances. Quick-dry technology reduces drying time by 30%.

## Air Quality

### Air Purifier
The HEPA Air Purifier covers rooms up to 400 sq ft with a 3-stage filtration system:

1. Pre-filter: Captures hair, dust, pet dander
2. True HEPA filter: 99.97% of particles 0.3 microns and larger
3. Activated carbon filter: Reduces odors, smoke, VOCs

**Features:** Auto mode adjusts fan speed based on air quality sensor. Sleep mode runs at 25dB — quieter than a whisper. Filter life indicator alerts when replacement is needed (every 6-8 months).

## Smart Home

### Thermostat
The Smart Thermostat learns your schedule and preferences over the first week and auto-adjusts. Energy reports show savings vs. previous months.

**Features:** WiFi connectivity, voice control (Alexa, Google, Siri), geofencing (auto-adjusts when you leave/arrive), 7-day programming, HVAC system alerts, compatible with 95% of heating/cooling systems.
```

- [ ] **Step 4: Create Markdown source for the fitness equipment guide**

Create `docs/product-catalog/fitness-equipment-guide.md`:

```markdown
# Fitness Equipment Guide

## Strength Training

### Dumbbells
The Adjustable Dumbbells use a quick-change mechanism to switch between 5 and 52.5 lbs per hand in 2.5 lb increments.

**Key specs:**
- Weight range: 5-52.5 lbs per dumbbell
- Increments: 2.5 lbs (5-25 lbs), 5 lbs (25-52.5 lbs)
- Mechanism: Dial selector — turn and lift
- Tray dimensions: 17" x 8" x 9"
- Material: Steel plates with nylon cradles
- Replaces 30 individual dumbbells

**Workout suggestions for beginners:**
- Bicep curls: Start at 10-15 lbs, 3 sets of 12 reps
- Shoulder press: Start at 10-15 lbs, 3 sets of 10 reps
- Goblet squats: Start at 20-25 lbs, 3 sets of 10 reps

### Pull-Up Bar
The Doorway Pull-Up Bar fits frames 26-36" wide with no screws or drilling. Supports up to 300 lbs with padded grips for comfort.

**Exercise options:** Pull-ups, chin-ups, hanging leg raises, neutral grip pull-ups.

### Resistance Bands
The Resistance Band Set includes 5 color-coded bands with varying resistance levels:
- Yellow: 5 lbs — warm-up, rehab
- Green: 10 lbs — light resistance
- Red: 15 lbs — medium resistance
- Blue: 20 lbs — heavy resistance
- Black: 25 lbs — maximum resistance

Bands can be combined for up to 75 lbs total. Kit includes: 2 handles, 2 ankle straps, door anchor, carry bag.

## Cardio

### Running Watch
The GPS Running Watch provides optical heart rate monitoring, customizable pace alerts, and a 14-day battery in smartwatch mode (25 hours in GPS mode).

**Features:** VO2 max estimate, training load tracking, recovery advisor, interval timer, breadcrumb navigation, 50m water resistance.

### Jump Rope
The Speed Jump Rope uses ball-bearing handles for smooth rotation. Adjustable steel cable (10ft) with anti-slip aluminum handles.

**Benefits:** Burns 10-16 calories per minute, improves coordination, strengthens calves and shoulders.

## Recovery

### Foam Roller
The High-Density Foam Roller (18" x 6") features a textured EVA surface for targeted muscle release.

**Recommended use:**
- IT band: Lie on side, roll from hip to knee, 30-60 seconds per side
- Upper back: Place roller perpendicular to spine, arms crossed, roll slowly
- Quads: Face down, roller under thighs, roll from hip to knee
- Calves: Sit with roller under calves, cross one leg over for added pressure

### Yoga Mat
The Yoga Mat 6mm uses TPE (thermoplastic elastomer) — eco-friendly, latex-free, and recyclable. Non-slip on both sides. Includes carrying strap.

**Dimensions:** 72" x 24" x 6mm. Weight: 2.5 lbs.

## Bags & Accessories

### Gym Bag
The Gym Duffel Bag (40L) features a ventilated shoe compartment, waterproof wet pocket, and padded shoulder strap.

**Dimensions:** 22" x 11" x 11". Material: 600D polyester with water-resistant coating.

### Cycling Gloves
Gel-padded palms reduce handlebar vibration and pressure on the ulnar nerve. Breathable mesh back with touchscreen-compatible fingertips. Pull tabs for easy removal.

### Hiking Backpack
The Hiking Backpack 40L includes an integrated rain cover, hydration bladder sleeve (bladder not included), adjustable hip belt with pockets, and a ventilated mesh back panel.

**Features:** Top-loading with front zip access, trekking pole attachments, compression straps, gear loops.

## Hydration

### Water Bottle
The Water Bottle 32oz uses double-wall vacuum insulation to keep drinks cold for 24 hours or hot for 12 hours. 18/8 stainless steel with BPA-free lid. Wide mouth fits standard ice cubes.
```

- [ ] **Step 5: Create Markdown source for the 5 hero product spec sheets**

Create `docs/product-catalog/laptop-pro-15-specs.md`:

```markdown
# Laptop Pro 15" — Technical Specifications

## Overview
A versatile 15.6" laptop built for productivity, development, and everyday use. Balances performance, battery life, and portability.

## Display
- Size: 15.6 inches
- Resolution: 1920 x 1080 (FHD)
- Panel: IPS, anti-glare
- Brightness: 300 nits
- Color: 72% NTSC / 100% sRGB
- Refresh rate: 60Hz

## Processor
- CPU: 12th Gen 10-core (6P + 4E)
- Base clock: 2.1 GHz
- Turbo clock: 4.7 GHz
- Cache: 12 MB Intel Smart Cache
- TDP: 45W

## Memory & Storage
- RAM: 16 GB DDR5-4800 (2 x 8 GB)
- Max RAM: 64 GB (2 SODIMM slots)
- Storage: 512 GB PCIe Gen4 NVMe M.2 SSD
- Additional slot: 1 x M.2 2280 (PCIe Gen4)

## Battery
- Capacity: 72 Wh, 4-cell lithium-polymer
- Runtime: Up to 10 hours (MobileMark 25)
- Charging: USB-C PD, 0-50% in 30 minutes with 65W adapter
- Adapter: 65W GaN USB-C (sold separately or use USB-C Fast Charger)

## Connectivity
- WiFi: WiFi 6E (802.11ax), tri-band
- Bluetooth: 5.3
- Ports: 2x USB-C (Thunderbolt 4), 1x USB-A 3.2, 1x HDMI 2.1, 1x 3.5mm audio, 1x SD card reader

## Physical
- Dimensions: 14.2" x 9.8" x 0.71" (W x D x H)
- Weight: 3.9 lbs (1.77 kg)
- Material: Aluminum lid, magnesium-alloy chassis
- Color: Space Gray
- Keyboard: Full-size with numpad, 1.5mm key travel, backlit
- Trackpad: 5.5" x 3.5" glass precision touchpad

## Webcam & Audio
- Camera: 1080p FHD with IR (Windows Hello)
- Microphone: Dual far-field mics with AI noise suppression
- Speakers: 2x 2W stereo, Dolby Atmos

## Certifications
- MIL-STD-810H (drop, shock, vibration, temperature)
- ENERGY STAR certified
- EPEAT Gold registered

## Warranty
- Standard: 1 year limited hardware warranty
- Extended: Available for 3 or 5 years
```

Create `docs/product-catalog/27-4k-monitor-specs.md`:

```markdown
# 27" 4K Monitor — Technical Specifications

## Overview
A professional-grade 27" 4K IPS monitor with USB-C connectivity and a fully adjustable ergonomic stand.

## Display
- Size: 27 inches
- Resolution: 3840 x 2160 (4K UHD)
- Pixel density: 163 PPI
- Panel: IPS
- Color: 99% sRGB, 95% DCI-P3
- Brightness: 350 nits (typical), 400 nits (peak)
- Contrast: 1300:1 (typical)
- Response time: 5ms (GtG)
- Refresh rate: 60Hz
- HDR: HDR400

## Connectivity
- 1x USB-C (DP Alt Mode + PD 65W)
- 1x HDMI 2.0
- 1x DisplayPort 1.4
- 2x USB-A 3.0 (downstream hub)
- 1x 3.5mm audio out

## Ergonomics
- Height adjustment: 150mm
- Tilt: -5° to 25°
- Swivel: ±45°
- Pivot: 90° (portrait mode)
- VESA mount: 100 x 100mm

## Physical
- Dimensions (with stand): 24.1" x 20.7" x 9.1"
- Weight (with stand): 14.8 lbs
- Weight (without stand): 9.5 lbs
- Bezel: 3-side borderless (6mm)

## Features
- Picture-by-Picture: View two sources simultaneously
- Blue light filter: Low blue light mode
- Flicker-free: DC dimming
- Cable management: Integrated clip on stand

## Power
- Consumption: 30W typical, 0.5W standby
- Power supply: Internal

## Warranty
- Standard: 3 years (panel, parts, labor)
- Dead pixel policy: Zero bright-dot guarantee
```

Create `docs/product-catalog/smartwatch-sport-specs.md`:

```markdown
# Smartwatch Sport — Technical Specifications

## Overview
A GPS sports watch with advanced health tracking, 7-day battery life, and 5ATM water resistance for swimming.

## Display
- Size: 1.4 inches
- Resolution: 454 x 454
- Type: AMOLED, always-on option
- Brightness: 1000 nits peak
- Glass: Gorilla Glass DX

## Health & Fitness
- Heart rate: Optical sensor, 24/7 monitoring
- SpO2: Blood oxygen measurement
- GPS: Dual-band (L1 + L5) for accuracy in urban canyons
- Sports modes: 150+ including running, cycling, swimming, hiking, skiing
- VO2 Max: Estimated from running workouts
- Sleep tracking: Sleep stages, sleep score, smart alarm
- Stress monitoring: All-day with guided breathing exercises

## Water Resistance
- Rating: 5ATM (50 meters)
- Swimming: Pool and open water tracking
- Shower safe: Yes

## Battery
- Smartwatch mode: 7 days
- GPS mode: 18 hours (dual-band), 36 hours (standard GPS)
- Always-on display: 4 days
- Charging: Magnetic pin, 0-100% in 65 minutes

## Connectivity
- Bluetooth: 5.2
- WiFi: 802.11 b/g/n
- NFC: Contactless payments
- Notifications: Call, text, app alerts with quick replies

## Physical
- Case: 46mm fiber-reinforced polymer with stainless steel bezel
- Weight: 52g (without strap)
- Strap: 22mm quick-release, silicone (included), compatible with standard 22mm bands
- Colors: Black, Olive, Slate Blue

## Compatibility
- iOS 14+ and Android 8+
- Companion app: Available on App Store and Google Play

## Warranty
- Standard: 2 years limited
```

Create `docs/product-catalog/robot-vacuum-specs.md`:

```markdown
# Robot Vacuum — Technical Specifications

## Overview
A LiDAR-navigated robot vacuum with self-emptying base station and app-controlled multi-room mapping.

## Cleaning Performance
- Suction: 2500Pa (3 levels: Quiet 1000Pa, Standard 1500Pa, Max 2500Pa)
- Brush: Main rubber brush + side brush
- Surface: Hard floors, low/medium pile carpet
- Edge cleaning: Dedicated side brush follows walls

## Navigation
- Technology: LiDAR 360° scanning
- Mapping: Real-time floor plan generation
- Multi-floor: Saves up to 4 floor maps
- Zones: No-go zones, virtual walls, room-specific cleaning schedules
- Obstacle avoidance: Front bumper + cliff sensors (6x)

## Dustbin & Base Station
- Onboard bin: 400ml
- Base station bin: 2.5L (auto-empties after each clean)
- Empty frequency: Every 30-60 days depending on usage
- Base dimensions: 14" x 12" x 16"

## Battery & Runtime
- Battery: 5200mAh lithium-ion
- Runtime: Up to 150 minutes (hard floor, Quiet mode)
- Auto-recharge: Returns to base when battery low, resumes where it left off
- Charging time: 4 hours (0-100%)

## Noise Level
- Quiet mode: 55 dB
- Standard mode: 60 dB
- Max mode: 67 dB

## Filtration
- Type: HEPA filter
- Efficiency: 99.97% of particles 0.3 microns and larger
- Replacement: Every 3-4 months

## Connectivity
- WiFi: 2.4 GHz
- App: iOS and Android
- Voice: Alexa, Google Assistant, Siri Shortcuts
- Scheduling: Per-room schedules with suction level presets

## Physical
- Dimensions: 13.8" diameter x 3.5" height
- Weight: 7.7 lbs
- Color: White

## Maintenance
- Main brush: Clean weekly, replace every 6-12 months
- Side brush: Replace every 3-6 months
- Filter: Replace every 3-4 months
- Sensors: Wipe with dry cloth monthly

## In the Box
- Robot vacuum
- Self-emptying base station
- 2x HEPA filters (1 installed + 1 spare)
- 1x extra side brush
- Power cord
- Quick start guide

## Warranty
- Standard: 1 year limited
- Battery: 6 months
```

Create `docs/product-catalog/stand-mixer-5qt-specs.md`:

```markdown
# Stand Mixer 5qt — Technical Specifications

## Overview
A tilt-head stand mixer with a 325-watt motor and planetary mixing action for thorough ingredient incorporation.

## Motor & Performance
- Motor: 325-watt DC motor
- Mixing action: Planetary — beater moves in one direction, shaft in the other
- Speeds: 10 (Stir, 2, 3, 4, 5, 6, 7, 8, 9, 10)
- Soft-start: Prevents ingredient splash at low speeds

## Bowl
- Capacity: 5 quarts (enough for 9 dozen cookies or 4 loaves of bread)
- Material: Polished stainless steel
- Handle: Comfortable grip for pouring
- Dishwasher safe: Yes

## Included Attachments
1. **Flat beater** — Cakes, cookies, frostings, mashed potatoes. Scrapes bowl automatically.
2. **Dough hook** — Bread, pizza dough, pasta. C-shaped for kneading.
3. **Wire whip** — Egg whites, whipped cream, meringue. 11-wire design.

## Attachment Hub
- Power takeoff: Front-mounted hub
- Compatible accessories (sold separately):
  - Pasta roller and cutter set
  - Food grinder
  - Spiralizer with peel, core, and slice
  - Ice cream maker
  - Grain mill
  - Sausage stuffer
  - Citrus juicer

## Design
- Head: Tilt-head for easy bowl and attachment access
- Lock: Locking mechanism keeps head secure during mixing
- Cord: 36" power cord with strain relief
- Feet: Non-slip rubber pads

## Physical
- Dimensions: 14" x 9" x 14" (H x W x D)
- Weight: 22 lbs
- Colors: White, Black, Red, Silver, Empire Blue
- Material: Die-cast zinc alloy body

## Electrical
- Power: 325 watts
- Voltage: 120V, 60Hz
- Plug: 3-prong grounded

## Safety
- Overload protection: Electronic motor cutoff
- Bowl locking: Twist-lock mechanism
- ETL listed

## Cleaning
- Bowl and attachments: Dishwasher safe (top rack for whip)
- Body: Wipe with damp cloth
- Hub cover: Removable, hand wash

## Warranty
- Standard: 2 years (motor, parts, labor)
- Motor: Hassle-free replacement if motor fails
```

- [ ] **Step 6: Create the PDF generation script**

Create `docs/product-catalog/generate-pdfs.py`:

```python
#!/usr/bin/env python3
"""Generate PDFs from Markdown source files in docs/product-catalog/.

Usage:
    pip install fpdf2
    python docs/product-catalog/generate-pdfs.py
"""

import re
from pathlib import Path

from fpdf import FPDF


def md_to_pdf(md_path: Path, pdf_path: Path) -> None:
    """Convert a Markdown file to a simple PDF."""
    pdf = FPDF()
    pdf.set_auto_page_break(auto=True, margin=20)
    pdf.add_page()

    text = md_path.read_text(encoding="utf-8")

    for line in text.split("\n"):
        stripped = line.strip()

        if stripped.startswith("# "):
            pdf.set_font("Helvetica", "B", 18)
            pdf.cell(0, 12, stripped[2:], new_x="LMARGIN", new_y="NEXT")
            pdf.ln(4)
        elif stripped.startswith("## "):
            pdf.set_font("Helvetica", "B", 14)
            pdf.ln(4)
            pdf.cell(0, 10, stripped[3:], new_x="LMARGIN", new_y="NEXT")
            pdf.ln(2)
        elif stripped.startswith("### "):
            pdf.set_font("Helvetica", "B", 12)
            pdf.ln(2)
            pdf.cell(0, 8, stripped[4:], new_x="LMARGIN", new_y="NEXT")
            pdf.ln(1)
        elif stripped.startswith("- ") or stripped.startswith("* "):
            pdf.set_font("Helvetica", "", 10)
            # Remove markdown bold markers for PDF
            content = re.sub(r"\*\*(.+?)\*\*", r"\1", stripped[2:])
            pdf.cell(8)  # indent
            pdf.multi_cell(0, 6, f"  \u2022 {content}")
        elif re.match(r"^\d+\.", stripped):
            pdf.set_font("Helvetica", "", 10)
            content = re.sub(r"\*\*(.+?)\*\*", r"\1", stripped)
            pdf.cell(8)  # indent
            pdf.multi_cell(0, 6, f"  {content}")
        elif stripped == "":
            pdf.ln(3)
        else:
            pdf.set_font("Helvetica", "", 10)
            content = re.sub(r"\*\*(.+?)\*\*", r"\1", stripped)
            pdf.multi_cell(0, 6, content)

    pdf.output(str(pdf_path))
    print(f"  {pdf_path.name} ({pdf.pages_count} pages)")


def main() -> None:
    catalog_dir = Path(__file__).parent
    md_files = sorted(catalog_dir.glob("*.md"))

    if not md_files:
        print("No Markdown files found in", catalog_dir)
        return

    print(f"Generating {len(md_files)} PDFs...")
    for md_file in md_files:
        pdf_file = md_file.with_suffix(".pdf")
        md_to_pdf(md_file, pdf_file)

    print("Done.")


if __name__ == "__main__":
    main()
```

- [ ] **Step 7: Generate the PDFs**

```bash
pip install fpdf2
python docs/product-catalog/generate-pdfs.py
```

Expected: 8 PDFs generated in `docs/product-catalog/`, each 1-3 pages.

- [ ] **Step 8: Verify PDFs are valid**

```bash
ls -la docs/product-catalog/*.pdf
# Verify all 8 exist and have non-zero size
file docs/product-catalog/*.pdf
# Should show "PDF document" for each
```

- [ ] **Step 9: Commit**

```bash
git add docs/product-catalog/
git commit -m "feat(catalog): add product PDFs for RAG document search

Create 3 buying guides and 5 spec sheets as Markdown sources with a
Python generation script. Product names match seed.sql for agent
correlation between catalog and document results."
```

---

### Task 3: Create PDF Seed Script

**Files:**
- Create: `scripts/seed-product-docs.sh`

- [ ] **Step 1: Create the seed script**

Create `scripts/seed-product-docs.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

# Seed product PDFs into the RAG ingestion service.
# Idempotent: skips if the product-docs collection already exists.
#
# Usage:
#   ./scripts/seed-product-docs.sh <ingestion-base-url> [jwt-token]
#
# Examples:
#   ./scripts/seed-product-docs.sh http://localhost:8001
#   ./scripts/seed-product-docs.sh http://localhost:8001 eyJhbGci...

INGESTION_URL="${1:?Usage: $0 <ingestion-base-url> [jwt-token]}"
JWT_TOKEN="${2:-}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PDF_DIR="$SCRIPT_DIR/../docs/product-catalog"
COLLECTION="product-docs"

# Build auth header if token provided
AUTH_HEADER=""
if [ -n "$JWT_TOKEN" ]; then
  AUTH_HEADER="Authorization: Bearer $JWT_TOKEN"
fi

curl_opts=(-s -f)
if [ -n "$AUTH_HEADER" ]; then
  curl_opts+=(-H "$AUTH_HEADER")
fi

# Check if collection already exists
echo "==> Checking for existing '$COLLECTION' collection..."
COLLECTIONS=$(curl "${curl_opts[@]}" "$INGESTION_URL/collections" 2>/dev/null || echo '{"collections":[]}')

if echo "$COLLECTIONS" | grep -q "\"name\":\"$COLLECTION\""; then
  echo "==> Collection '$COLLECTION' already exists, skipping seed."
  exit 0
fi

# Upload each PDF
PDF_COUNT=0
for pdf in "$PDF_DIR"/*.pdf; do
  [ -f "$pdf" ] || continue
  FILENAME=$(basename "$pdf")
  echo "==> Uploading $FILENAME to collection '$COLLECTION'..."

  RESPONSE=$(curl "${curl_opts[@]}" \
    -X POST \
    -F "file=@$pdf" \
    "$INGESTION_URL/ingest?collection=$COLLECTION" 2>&1) || {
    echo "    WARN: Failed to upload $FILENAME (may be rate-limited), retrying in 15s..."
    sleep 15
    RESPONSE=$(curl "${curl_opts[@]}" \
      -X POST \
      -F "file=@$pdf" \
      "$INGESTION_URL/ingest?collection=$COLLECTION")
  }

  CHUNKS=$(echo "$RESPONSE" | grep -o '"chunks_created":[0-9]*' | cut -d: -f2 || echo "?")
  echo "    OK: $CHUNKS chunks created"
  PDF_COUNT=$((PDF_COUNT + 1))

  # Rate limit: ingestion API allows 5 requests/minute
  if [ "$PDF_COUNT" -lt "$(ls "$PDF_DIR"/*.pdf 2>/dev/null | wc -l)" ]; then
    echo "    (waiting 13s for rate limit...)"
    sleep 13
  fi
done

echo "==> Seeded $PDF_COUNT PDFs into collection '$COLLECTION'."
```

- [ ] **Step 2: Make it executable**

```bash
chmod +x scripts/seed-product-docs.sh
```

- [ ] **Step 3: Commit**

```bash
git add scripts/seed-product-docs.sh
git commit -m "feat(deploy): add PDF seed script for product docs RAG collection

Idempotent script that uploads product PDFs to the ingestion service.
Handles rate limiting (5 req/min) with 13s delays between uploads."
```

---

### Task 4: Add Seed Step to Deploy Script

**Files:**
- Modify: `k8s/deploy.sh`

- [ ] **Step 1: Read the deploy script to find the right insertion point**

Read `k8s/deploy.sh` to find where AI services are deployed and confirmed available. The seed step should run after `kubectl wait` confirms the ingestion deployment is ready.

- [ ] **Step 2: Add the seed step after AI services are ready**

In `k8s/deploy.sh`, after the line that waits for the ingestion service to be available in the production deploy flow, add:

```bash
echo "==> Seeding product documents for RAG..."
"$REPO_DIR/scripts/seed-product-docs.sh" "http://$(minikube ip):80/ingestion" || echo "WARN: Product doc seeding failed (non-fatal)"
```

Use `|| echo "WARN: ..."` so a seed failure doesn't block the rest of the deploy. The script is idempotent so re-running deploy won't duplicate data.

Also add the same step in the QA deploy flow, using the QA ingestion URL.

- [ ] **Step 3: Commit**

```bash
git add k8s/deploy.sh
git commit -m "feat(deploy): run product doc seeding after AI services deploy"
```

---

### Task 5: Create ToolResultDisplay Router Component

**Files:**
- Create: `frontend/src/components/go/tool-results/ToolResultDisplay.tsx`
- Create: `frontend/src/components/go/tool-results/types.ts`

- [ ] **Step 1: Read Next.js docs for current component patterns**

Check `node_modules/next/dist/docs/` for any relevant changes to component patterns, especially around client components.

- [ ] **Step 2: Create the shared types file**

Create `frontend/src/components/go/tool-results/types.ts`:

```typescript
// Types for the display payloads returned by the Go ai-service.
// Each tool result has a `kind` discriminator field.

export type ProductItem = {
  id: string;
  name: string;
  price: number; // cents
  stock?: number;
  category?: string;
};

export type CartItem = {
  id: string;
  product_id: string;
  product_name: string;
  product_price: number; // cents
  quantity: number;
};

export type OrderSummary = {
  id: string;
  status: string;
  total: number; // cents
  created_at: string;
};

export type SearchChunk = {
  text: string;
  filename: string;
  page_number: number;
  score: number;
};

export type RagSource = {
  file: string;
  page: number;
};

// Discriminated union of all display payload shapes
export type ToolDisplay =
  | { kind: "product_list"; products: ProductItem[] }
  | { kind: "product_card"; product: ProductItem }
  | { kind: "cart"; cart: { items: CartItem[]; total: number } }
  | { kind: "cart_item"; item: CartItem }
  | { kind: "search_results"; results: SearchChunk[] }
  | { kind: "rag_answer"; answer: string; sources: RagSource[] }
  | { kind: "order_list"; orders: OrderSummary[] }
  | { kind: "order_card"; order: OrderSummary }
  | { kind: "inventory"; product_id: string; stock: number; in_stock: boolean }
  | { kind: "collections_list"; collections: { name: string; point_count: number }[] }
  | { kind: "return_confirmation"; return: { id: string; order_id: string; status: string; reason: string } };

// Tools that query the product catalog (database)
export const CATALOG_TOOLS = new Set([
  "search_products",
  "get_product",
  "check_inventory",
  "view_cart",
  "add_to_cart",
  "list_orders",
  "get_order",
  "summarize_orders",
  "initiate_return",
]);

// Tools that query RAG document knowledge
export const RAG_TOOLS = new Set([
  "search_documents",
  "ask_document",
  "list_collections",
]);
```

- [ ] **Step 3: Create the router component**

Create `frontend/src/components/go/tool-results/ToolResultDisplay.tsx`:

```typescript
"use client";

import { type ToolDisplay, CATALOG_TOOLS, RAG_TOOLS } from "./types";
import { ProductListResult } from "./ProductListResult";
import { ProductCardResult } from "./ProductCardResult";
import { CartResult } from "./CartResult";
import { CartItemResult } from "./CartItemResult";
import { SearchResultsResult } from "./SearchResultsResult";
import { RagAnswerResult } from "./RagAnswerResult";
import { OrderListResult } from "./OrderListResult";
import { OrderCardResult } from "./OrderCardResult";
import { InventoryResult } from "./InventoryResult";
import { CollectionsResult } from "./CollectionsResult";
import { ReturnResult } from "./ReturnResult";

type Props = {
  toolName: string;
  display: unknown;
};

function isToolDisplay(d: unknown): d is ToolDisplay {
  return typeof d === "object" && d !== null && "kind" in d;
}

function SourceLabel({ toolName }: { toolName: string }) {
  if (CATALOG_TOOLS.has(toolName)) {
    return (
      <div className="mb-1 flex items-center gap-1 text-[10px] font-semibold uppercase text-blue-500">
        <span aria-hidden>📦</span> Catalog Search
      </div>
    );
  }
  if (RAG_TOOLS.has(toolName)) {
    return (
      <div className="mb-1 flex items-center gap-1 text-[10px] font-semibold uppercase text-green-500">
        <span aria-hidden>📄</span> Product Knowledge
      </div>
    );
  }
  return null;
}

export function ToolResultDisplay({ toolName, display }: Props) {
  if (!isToolDisplay(display)) {
    // Fallback: formatted JSON for unknown display shapes
    return (
      <div className="rounded-lg border border-dashed p-3">
        <div className="mb-1 text-xs font-semibold text-muted-foreground">
          {toolName}
        </div>
        <pre className="max-h-48 overflow-auto text-xs text-muted-foreground">
          {JSON.stringify(display, null, 2)}
        </pre>
      </div>
    );
  }

  const borderClass = CATALOG_TOOLS.has(toolName)
    ? "border-l-blue-500"
    : RAG_TOOLS.has(toolName)
      ? "border-l-green-500"
      : "border-l-muted";

  return (
    <div className={`rounded-lg border-l-[3px] bg-muted/50 ${borderClass}`}>
      <div className="p-3">
        <SourceLabel toolName={toolName} />
        <DisplayContent display={display} />
      </div>
    </div>
  );
}

function DisplayContent({ display }: { display: ToolDisplay }) {
  switch (display.kind) {
    case "product_list":
      return <ProductListResult products={display.products} />;
    case "product_card":
      return <ProductCardResult product={display.product} />;
    case "cart":
      return <CartResult cart={display.cart} />;
    case "cart_item":
      return <CartItemResult item={display.item} />;
    case "search_results":
      return <SearchResultsResult results={display.results} />;
    case "rag_answer":
      return <RagAnswerResult answer={display.answer} sources={display.sources} />;
    case "order_list":
      return <OrderListResult orders={display.orders} />;
    case "order_card":
      return <OrderCardResult order={display.order} />;
    case "inventory":
      return <InventoryResult productId={display.product_id} stock={display.stock} inStock={display.in_stock} />;
    case "collections_list":
      return <CollectionsResult collections={display.collections} />;
    case "return_confirmation":
      return <ReturnResult ret={display.return} />;
    default: {
      // Exhaustive check — if TypeScript complains here, a new kind was added but not handled
      const _exhaustive: never = display;
      return (
        <pre className="text-xs text-muted-foreground">
          {JSON.stringify(_exhaustive, null, 2)}
        </pre>
      );
    }
  }
}
```

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/go/tool-results/
git commit -m "feat(ui): add ToolResultDisplay router with types and source labels

Routes display payloads by kind to typed components. Blue border for
catalog tools, green for RAG tools. Falls back to JSON for unknown kinds."
```

---

### Task 6: Create Catalog Tool Result Components

**Files:**
- Create: `frontend/src/components/go/tool-results/ProductListResult.tsx`
- Create: `frontend/src/components/go/tool-results/ProductCardResult.tsx`
- Create: `frontend/src/components/go/tool-results/CartResult.tsx`
- Create: `frontend/src/components/go/tool-results/CartItemResult.tsx`
- Create: `frontend/src/components/go/tool-results/InventoryResult.tsx`

- [ ] **Step 1: Create ProductListResult**

Create `frontend/src/components/go/tool-results/ProductListResult.tsx`:

```typescript
import type { ProductItem } from "./types";

function formatPrice(cents: number): string {
  return `$${(cents / 100).toFixed(2)}`;
}

export function ProductListResult({ products }: { products: ProductItem[] }) {
  if (products.length === 0) {
    return <p className="text-xs text-muted-foreground">No products found.</p>;
  }

  return (
    <div>
      <p className="mb-2 text-[10px] text-muted-foreground">
        {products.length} product{products.length !== 1 ? "s" : ""} found
      </p>
      <div className="divide-y divide-border">
        {products.map((p) => (
          <div key={p.id} className="flex items-center gap-3 py-2">
            <div className="flex h-8 w-8 items-center justify-center rounded bg-muted text-sm">
              🛒
            </div>
            <div className="flex-1 min-w-0">
              <div className="truncate text-xs font-semibold">{p.name}</div>
              {p.category && (
                <div className="text-[10px] text-muted-foreground">
                  {p.category}
                </div>
              )}
            </div>
            <div className="text-sm font-semibold text-green-600">
              {formatPrice(p.price)}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Create ProductCardResult**

Create `frontend/src/components/go/tool-results/ProductCardResult.tsx`:

```typescript
import type { ProductItem } from "./types";

function formatPrice(cents: number): string {
  return `$${(cents / 100).toFixed(2)}`;
}

export function ProductCardResult({ product }: { product: ProductItem }) {
  return (
    <div>
      <div className="text-sm font-semibold">{product.name}</div>
      {product.category && (
        <div className="text-[10px] uppercase text-muted-foreground">
          {product.category}
        </div>
      )}
      <div className="mt-1 text-lg font-bold text-green-600">
        {formatPrice(product.price)}
      </div>
      {product.stock !== undefined && (
        <div className="mt-1 text-[10px] text-muted-foreground">
          {product.stock > 0
            ? `${product.stock} in stock`
            : "Out of stock"}
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 3: Create CartResult**

Create `frontend/src/components/go/tool-results/CartResult.tsx`:

```typescript
import type { CartItem } from "./types";

function formatPrice(cents: number): string {
  return `$${(cents / 100).toFixed(2)}`;
}

export function CartResult({
  cart,
}: {
  cart: { items: CartItem[]; total: number };
}) {
  if (cart.items.length === 0) {
    return <p className="text-xs text-muted-foreground">Your cart is empty.</p>;
  }

  return (
    <div>
      <div className="divide-y divide-border">
        {cart.items.map((item) => (
          <div key={item.id} className="flex items-center justify-between py-2">
            <div className="min-w-0 flex-1">
              <div className="truncate text-xs font-semibold">
                {item.product_name}
              </div>
              <div className="text-[10px] text-muted-foreground">
                Qty: {item.quantity} × {formatPrice(item.product_price)}
              </div>
            </div>
            <div className="text-xs font-semibold">
              {formatPrice(item.product_price * item.quantity)}
            </div>
          </div>
        ))}
      </div>
      <div className="mt-2 flex justify-between border-t pt-2">
        <span className="text-xs font-semibold">Total</span>
        <span className="text-sm font-bold text-green-600">
          {formatPrice(cart.total)}
        </span>
      </div>
    </div>
  );
}
```

- [ ] **Step 4: Create CartItemResult**

Create `frontend/src/components/go/tool-results/CartItemResult.tsx`:

```typescript
import type { CartItem } from "./types";

function formatPrice(cents: number): string {
  return `$${(cents / 100).toFixed(2)}`;
}

export function CartItemResult({ item }: { item: CartItem }) {
  return (
    <div className="flex items-center gap-2">
      <span className="text-sm" aria-hidden>
        ✅
      </span>
      <div>
        <div className="text-xs font-semibold">
          Added {item.product_name} to cart
        </div>
        <div className="text-[10px] text-muted-foreground">
          Qty: {item.quantity} — {formatPrice(item.product_price)}
        </div>
      </div>
    </div>
  );
}
```

- [ ] **Step 5: Create InventoryResult**

Create `frontend/src/components/go/tool-results/InventoryResult.tsx`:

```typescript
export function InventoryResult({
  productId: _productId,
  stock,
  inStock,
}: {
  productId: string;
  stock: number;
  inStock: boolean;
}) {
  return (
    <div className="flex items-center gap-2">
      <span className="text-sm" aria-hidden>
        {inStock ? "✅" : "❌"}
      </span>
      <div className="text-xs">
        {inStock ? (
          <span>
            <span className="font-semibold text-green-600">{stock}</span> in
            stock
          </span>
        ) : (
          <span className="font-semibold text-red-600">Out of stock</span>
        )}
      </div>
    </div>
  );
}
```

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/go/tool-results/ProductListResult.tsx \
        frontend/src/components/go/tool-results/ProductCardResult.tsx \
        frontend/src/components/go/tool-results/CartResult.tsx \
        frontend/src/components/go/tool-results/CartItemResult.tsx \
        frontend/src/components/go/tool-results/InventoryResult.tsx
git commit -m "feat(ui): add catalog tool result components

Product list/card, cart, cart item, and inventory display components
with formatted prices and stock indicators."
```

---

### Task 7: Create RAG Tool Result Components

**Files:**
- Create: `frontend/src/components/go/tool-results/SearchResultsResult.tsx`
- Create: `frontend/src/components/go/tool-results/RagAnswerResult.tsx`
- Create: `frontend/src/components/go/tool-results/CollectionsResult.tsx`

- [ ] **Step 1: Create SearchResultsResult**

Create `frontend/src/components/go/tool-results/SearchResultsResult.tsx`:

```typescript
import type { SearchChunk } from "./types";

export function SearchResultsResult({
  results,
}: {
  results: SearchChunk[];
}) {
  if (results.length === 0) {
    return (
      <p className="text-xs text-muted-foreground">
        No matching documents found.
      </p>
    );
  }

  return (
    <div className="space-y-2">
      {results.map((r, i) => (
        <div key={i} className="rounded border bg-background p-2">
          <p className="text-xs leading-relaxed line-clamp-3">{r.text}</p>
          <div className="mt-1 flex flex-wrap gap-1">
            <span className="rounded bg-blue-950 px-2 py-0.5 text-[10px] text-blue-300">
              📄 {r.filename}, p.{r.page_number}
            </span>
            <span className="rounded bg-muted px-2 py-0.5 text-[10px] text-muted-foreground">
              {(r.score * 100).toFixed(0)}% match
            </span>
          </div>
        </div>
      ))}
    </div>
  );
}
```

- [ ] **Step 2: Create RagAnswerResult**

Create `frontend/src/components/go/tool-results/RagAnswerResult.tsx`:

```typescript
import type { RagSource } from "./types";

export function RagAnswerResult({
  answer,
  sources,
}: {
  answer: string;
  sources: RagSource[];
}) {
  return (
    <div>
      <p className="text-xs leading-relaxed">{answer}</p>
      {sources.length > 0 && (
        <div className="mt-2 flex flex-wrap gap-1">
          {sources.map((s, i) => (
            <span
              key={i}
              className="rounded bg-blue-950 px-2 py-0.5 text-[10px] text-blue-300"
            >
              📄 {s.file}, p.{s.page}
            </span>
          ))}
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 3: Create CollectionsResult**

Create `frontend/src/components/go/tool-results/CollectionsResult.tsx`:

```typescript
export function CollectionsResult({
  collections,
}: {
  collections: { name: string; point_count: number }[];
}) {
  if (collections.length === 0) {
    return (
      <p className="text-xs text-muted-foreground">No collections found.</p>
    );
  }

  return (
    <div className="divide-y divide-border">
      {collections.map((c) => (
        <div key={c.name} className="flex items-center justify-between py-1.5">
          <span className="text-xs font-semibold">{c.name}</span>
          <span className="text-[10px] text-muted-foreground">
            {c.point_count} chunks
          </span>
        </div>
      ))}
    </div>
  );
}
```

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/go/tool-results/SearchResultsResult.tsx \
        frontend/src/components/go/tool-results/RagAnswerResult.tsx \
        frontend/src/components/go/tool-results/CollectionsResult.tsx
git commit -m "feat(ui): add RAG tool result components

Search results with source badges and match scores, RAG answers with
citation badges, and collections list with chunk counts."
```

---

### Task 8: Create Order and Return Components

**Files:**
- Create: `frontend/src/components/go/tool-results/OrderListResult.tsx`
- Create: `frontend/src/components/go/tool-results/OrderCardResult.tsx`
- Create: `frontend/src/components/go/tool-results/ReturnResult.tsx`

- [ ] **Step 1: Create OrderListResult**

Create `frontend/src/components/go/tool-results/OrderListResult.tsx`:

```typescript
import type { OrderSummary } from "./types";

function formatPrice(cents: number): string {
  return `$${(cents / 100).toFixed(2)}`;
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
}

const STATUS_COLORS: Record<string, string> = {
  pending: "bg-yellow-900 text-yellow-300",
  confirmed: "bg-blue-900 text-blue-300",
  shipped: "bg-purple-900 text-purple-300",
  delivered: "bg-green-900 text-green-300",
  cancelled: "bg-red-900 text-red-300",
  returned: "bg-gray-800 text-gray-300",
};

export function OrderListResult({ orders }: { orders: OrderSummary[] }) {
  if (orders.length === 0) {
    return <p className="text-xs text-muted-foreground">No orders found.</p>;
  }

  return (
    <div className="divide-y divide-border">
      {orders.map((o) => (
        <div key={o.id} className="flex items-center justify-between py-2">
          <div>
            <div className="text-xs font-semibold">
              Order #{o.id.slice(0, 8)}
            </div>
            <div className="text-[10px] text-muted-foreground">
              {formatDate(o.created_at)}
            </div>
          </div>
          <div className="flex items-center gap-2">
            <span
              className={`rounded px-2 py-0.5 text-[10px] ${STATUS_COLORS[o.status] ?? "bg-muted text-muted-foreground"}`}
            >
              {o.status}
            </span>
            <span className="text-xs font-semibold">
              {formatPrice(o.total)}
            </span>
          </div>
        </div>
      ))}
    </div>
  );
}
```

- [ ] **Step 2: Create OrderCardResult**

Create `frontend/src/components/go/tool-results/OrderCardResult.tsx`:

```typescript
import type { OrderSummary } from "./types";

function formatPrice(cents: number): string {
  return `$${(cents / 100).toFixed(2)}`;
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
}

export function OrderCardResult({ order }: { order: OrderSummary }) {
  return (
    <div>
      <div className="text-xs font-semibold">Order #{order.id.slice(0, 8)}</div>
      <div className="mt-1 space-y-1 text-[10px] text-muted-foreground">
        <div>
          Status: <span className="font-semibold text-foreground">{order.status}</span>
        </div>
        <div>
          Total: <span className="font-semibold text-green-600">{formatPrice(order.total)}</span>
        </div>
        <div>Placed: {formatDate(order.created_at)}</div>
      </div>
    </div>
  );
}
```

- [ ] **Step 3: Create ReturnResult**

Create `frontend/src/components/go/tool-results/ReturnResult.tsx`:

```typescript
export function ReturnResult({
  ret,
}: {
  ret: { id: string; order_id: string; status: string; reason: string };
}) {
  return (
    <div>
      <div className="flex items-center gap-2">
        <span className="text-sm" aria-hidden>
          ↩️
        </span>
        <span className="text-xs font-semibold">Return Initiated</span>
      </div>
      <div className="mt-1 space-y-1 text-[10px] text-muted-foreground">
        <div>Return ID: {ret.id.slice(0, 8)}</div>
        <div>Order: #{ret.order_id.slice(0, 8)}</div>
        <div>
          Status: <span className="font-semibold text-foreground">{ret.status}</span>
        </div>
        <div>Reason: {ret.reason}</div>
      </div>
    </div>
  );
}
```

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/go/tool-results/OrderListResult.tsx \
        frontend/src/components/go/tool-results/OrderCardResult.tsx \
        frontend/src/components/go/tool-results/ReturnResult.tsx
git commit -m "feat(ui): add order and return tool result components

Order list with status badges, order detail card, and return
confirmation display."
```

---

### Task 9: Wire ToolResultDisplay into AiAssistantDrawer

**Files:**
- Modify: `frontend/src/components/go/AiAssistantDrawer.tsx`

- [ ] **Step 1: Replace AiToolCallCard usage with ToolResultDisplay**

In `AiAssistantDrawer.tsx`, make these changes:

1. Replace the `AiToolCallCard` import with `ToolResultDisplay`:

```typescript
// Remove this line:
import { AiToolCallCard, type ToolCallView } from "./AiToolCallCard";

// Add this line:
import { ToolResultDisplay } from "./tool-results/ToolResultDisplay";
```

2. Keep the `ToolCallView` type inline (it's used internally for state management):

```typescript
type ToolCallView = {
  id: string;
  name: string;
  args: unknown;
  status: "running" | "success" | "error";
  display?: unknown;
  error?: string;
};
```

3. Update the tool rendering section (around line 161) from:

```typescript
{it.kind === "tool" && <AiToolCallCard call={it.call} />}
```

to:

```typescript
{it.kind === "tool" && (
  <>
    {it.call.status === "running" && (
      <div className="flex items-center gap-1.5 text-[10px] text-muted-foreground">
        <span className="inline-block h-1.5 w-1.5 animate-pulse rounded-full bg-yellow-500" />
        {it.call.name}
      </div>
    )}
    {it.call.status === "error" && (
      <div className="rounded border border-red-500/30 p-2 text-xs text-red-500">
        <span className="font-semibold">{it.call.name}</span>: {it.call.error}
      </div>
    )}
    {it.call.status === "success" && it.call.display !== undefined && (
      <ToolResultDisplay toolName={it.call.name} display={it.call.display} />
    )}
  </>
)}
```

- [ ] **Step 2: Run frontend preflight**

Run: `make preflight-frontend`
Expected: PASS (tsc + lint)

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/go/AiAssistantDrawer.tsx
git commit -m "feat(ui): wire rich tool result rendering into shopping assistant

Replace raw JSON tool cards with typed ToolResultDisplay components.
Running tools show animated indicator, errors show inline, success
shows rich typed content with source labels."
```

---

### Task 10: Add Collapsible Context Panel to Drawer

**Files:**
- Modify: `frontend/src/components/go/AiAssistantDrawer.tsx`

- [ ] **Step 1: Add context panel state and component**

In `AiAssistantDrawer.tsx`, add a `showPanel` state variable:

```typescript
const [showPanel, setShowPanel] = useState(true);
```

Auto-collapse the panel when the first message is sent. In the `handleSend` function, after `setBusy(true)`, add:

```typescript
setShowPanel(false);
```

- [ ] **Step 2: Add the context panel JSX**

After the `<header>` element and before the `<ScrollArea>`, add:

```typescript
{showPanel && (
  <div className="border-b px-4 py-3">
    <button
      type="button"
      onClick={() => setShowPanel(false)}
      className="mb-2 flex w-full items-center justify-between text-xs font-semibold text-muted-foreground"
    >
      <span>What can I help with?</span>
      <span className="text-[10px]">✕</span>
    </button>
    <div className="grid grid-cols-2 gap-2">
      <div className="rounded-md bg-muted p-2">
        <div className="text-[10px] font-semibold text-blue-500">
          📦 PRODUCT CATALOG
        </div>
        <p className="mt-1 text-[10px] text-muted-foreground">
          Search products, check prices & stock, manage your cart, view orders
        </p>
      </div>
      <div className="rounded-md bg-muted p-2">
        <div className="text-[10px] font-semibold text-green-500">
          📄 PRODUCT KNOWLEDGE
        </div>
        <p className="mt-1 text-[10px] text-muted-foreground">
          Spec sheets, buying guides, compatibility info
        </p>
        <a
          href="/ai/rag"
          className="mt-1 block text-[10px] text-blue-400 hover:underline"
        >
          Add docs at AI / Document Q&A →
        </a>
      </div>
    </div>
    <div className="mt-2">
      <div className="text-[10px] text-muted-foreground">TRY ASKING:</div>
      <div className="mt-1 flex flex-wrap gap-1">
        {[
          "Compare laptops under $1000",
          "What's the battery life of the Laptop Pro 15?",
          "Which cookware is oven-safe?",
        ].map((q) => (
          <button
            key={q}
            type="button"
            className="rounded-full bg-muted px-2.5 py-1 text-[10px] text-muted-foreground hover:bg-muted/80"
            onClick={() => {
              setInput(q);
              setShowPanel(false);
            }}
          >
            &quot;{q}&quot;
          </button>
        ))}
      </div>
    </div>
  </div>
)}
```

- [ ] **Step 3: Remove the old placeholder text**

Remove the existing empty-state text (around line 141-144):

```typescript
// Remove this block:
{items.length === 0 && (
  <p className="text-sm text-muted-foreground">
    Try: &quot;find me a waterproof jacket under $150&quot; or
    &quot;where&apos;s my last order?&quot;
  </p>
)}
```

- [ ] **Step 4: Run frontend preflight**

Run: `make preflight-frontend`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/go/AiAssistantDrawer.tsx
git commit -m "feat(ui): add collapsible context panel to shopping assistant

Two-column panel shows data sources (catalog vs product knowledge),
link to document upload page, and clickable sample questions.
Auto-collapses on first message."
```

---

### Task 11: Update E2E Tests

**Files:**
- Modify: `frontend/e2e/mocked/go-ai-assistant.spec.ts`

- [ ] **Step 1: Update existing test for rich rendering**

The existing test mocks a `tool_result` with `display: { kind: "product_list", products: [...] }`. After our changes, this should render the `ProductListResult` component instead of raw JSON. Update the assertion:

In `go-ai-assistant.spec.ts`, replace the tool args assertion:

```typescript
// Remove this line (tool args are no longer shown as JSON):
await expect(page.getByText(/"max_price": 150/)).toBeVisible();

// Replace with: the product name renders in the rich component
await expect(page.getByText("Waterproof Jacket")).toBeVisible();
```

Also verify the source label appears:

```typescript
// Catalog source label should be visible
await expect(page.getByText("Catalog Search")).toBeVisible();
```

- [ ] **Step 2: Add test for RAG tool result rendering**

Add a new test that mocks a RAG tool call:

```typescript
test("renders RAG search results with source badges", async ({ page }) => {
  await page.route("**/chat", (route) => {
    const sseBody = [
      "event: tool_call",
      'data: {"name":"search_documents","args":{"query":"battery life laptop"}}',
      "",
      "event: tool_result",
      'data: {"name":"search_documents","display":{"kind":"search_results","results":[{"text":"The Laptop Pro 15 features a 72Wh battery rated for up to 10 hours.","filename":"laptop-pro-15-specs.pdf","page_number":1,"score":0.92}]}}',
      "",
      "event: final",
      'data: {"text":"The Laptop Pro 15 has up to 10 hours of battery life."}',
      "",
    ].join("\n");

    return route.fulfill({
      status: 200,
      contentType: "text/event-stream",
      body: sseBody,
    });
  });

  await page.route("**/products**", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ products: [], total: 0, page: 1, limit: 20 }),
    }),
  );

  await page.goto("/go/ecommerce");
  await page.getByTestId("ai-assistant-open").click();
  await page.getByTestId("ai-assistant-input").fill("battery life laptop");
  await page.getByTestId("ai-assistant-send").click();

  // RAG source label
  await expect(page.getByText("Product Knowledge")).toBeVisible();

  // Source badge with filename and page
  await expect(page.getByText("laptop-pro-15-specs.pdf, p.1")).toBeVisible();

  // Match score
  await expect(page.getByText("92% match")).toBeVisible();

  // Final answer
  await expect(page.getByTestId("ai-assistant-final")).toHaveText(
    "The Laptop Pro 15 has up to 10 hours of battery life.",
  );
});
```

- [ ] **Step 3: Add test for context panel and sample questions**

```typescript
test("shows context panel with sample questions", async ({ page }) => {
  await page.route("**/products**", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ products: [], total: 0, page: 1, limit: 20 }),
    }),
  );

  await page.goto("/go/ecommerce");
  await page.getByTestId("ai-assistant-open").click();

  // Context panel visible
  await expect(page.getByText("What can I help with?")).toBeVisible();
  await expect(page.getByText("Product Catalog", { exact: false })).toBeVisible();
  await expect(page.getByText("Product Knowledge", { exact: false })).toBeVisible();

  // Sample questions visible
  await expect(page.getByText("Compare laptops under $1000")).toBeVisible();

  // Link to RAG page
  await expect(page.getByText("Add docs at AI / Document Q&A")).toBeVisible();
});
```

- [ ] **Step 4: Run E2E tests**

Run: `make preflight-e2e`
Expected: All tests pass, including the new ones.

- [ ] **Step 5: Commit**

```bash
git add frontend/e2e/mocked/go-ai-assistant.spec.ts
git commit -m "test(e2e): update AI assistant tests for rich rendering

Verify product list rendering, RAG search result badges with scores,
source labels (Catalog vs Product Knowledge), and context panel with
sample questions."
```

---

### Task 12: Run Full Preflight and Final Commit

- [ ] **Step 1: Run full preflight**

```bash
make preflight
```

Expected: All checks pass (Python, frontend, security, Java, Go).

- [ ] **Step 2: Fix any lint or type errors**

If `tsc` or `eslint` flag issues in the new components, fix them before proceeding.

- [ ] **Step 3: Push the branch**

```bash
git push -u origin agent/feat-unified-ai-assistant
```

- [ ] **Step 4: Watch CI**

Monitor the GitHub Actions run. Wait for all checks to complete. If CI fails, debug and push fixes.

- [ ] **Step 5: Create PR to qa**

Once CI is green, create a PR from `agent/feat-unified-ai-assistant` to `qa`.
