# GalaxyVoyagers Portfolio Section Design

## Goal

Add GalaxyVoyagers.com to the portfolio frontend as a high-visibility deployed
project, while keeping the landing page concise and moving the deeper technical
explanation to a dedicated `/galaxy` route.

## Landing Page

The root portfolio page will add a new first card titled
`GalaxyVoyagers.com`. The card will match the existing portfolio card pattern:
title, short description, and a concise body paragraph.

The root card should act as a teaser, not the full case study. It will mention
that GalaxyVoyagers is a deployed collaborative sci-fi worldbuilding platform
built with Next.js, Go, GraphQL, gRPC, PostgreSQL, MongoDB, Redis, RabbitMQ, and
AI-assisted generation. The card links to `/galaxy` so visitors are encouraged
to view the architecture walkthrough before leaving the portfolio site.

## Galaxy Route

The `/galaxy` page will provide the full project explanation. It will include:

- A page heading for `GalaxyVoyagers.com`.
- A prominent external link to `https://galaxyvoyagers.com`.
- A full description of the product and engineering purpose.
- A technology stack section covering Next.js, Apollo Client, Go, gqlgen,
  GraphQL subscriptions, gRPC, PostgreSQL, MongoDB, Redis, RabbitMQ, OpenAI,
  image generation, Docker, GitHub Actions, Kubernetes, and Cloudflare Tunnel
  where supported by the Story repo.
- A Mermaid architecture diagram modeled after the existing Go microservices
  diagram.
- A focused section explaining why GraphQL was the right API boundary.

## Architecture Diagram

The diagram will show the deployed request path and service boundaries:

- Browser / Next.js frontend using Apollo Client.
- Go GraphQL gateway using gqlgen.
- gRPC backend services for story, chat, auth, image, story generation, and
  Stripe.
- Datastores: PostgreSQL, MongoDB, Redis.
- Async infrastructure: RabbitMQ for image-generation jobs and streaming work.
- AI providers/workers for story and image generation.
- Kubernetes/Cloudflare deployment boundary where useful for the reader.

The diagram should make the gateway's role clear: the browser sends GraphQL
queries and subscriptions to one entry point, while the gateway resolves nested
fields through backend services and storage systems.

## Why GraphQL

The GraphQL section will explain the project-specific fit:

GalaxyVoyagers has highly connected domain objects: stories, scenes,
characters, locations, organizations, conflicts, roles, ships, images, posts,
and comments. A reader often needs a nested view of this graph, such as a story
with scenes, each scene's characters and locations, related images, and
discussion content.

With a REST-only browser API, the frontend would commonly need chained requests
to fetch the primary object, then related records, then media or discussion
data. GraphQL lets the frontend ask for the exact nested shape in one operation.
The gateway owns the complexity of resolving those fields across services and
datastores, reducing browser round trips and keeping the UI data contract
explicit.

This section should avoid claiming GraphQL is universally better. The point is
that it fits this domain because the UI frequently traverses connected
worldbuilding data and benefits from declarative nested queries.

## Components And Reuse

The implementation will reuse existing frontend patterns:

- `frontend/src/app/page.tsx` for the landing page card list.
- A new `frontend/src/app/galaxy/page.tsx` route.
- Existing shadcn card styling for the root card.
- Existing `MermaidDiagram` component for the architecture diagram.
- Existing typography, spacing, and link patterns from `/go` and other
  portfolio pages.

No new runtime dependency is expected.

## Testing

Verification will run the frontend preflight required by the repo:

- `make preflight-frontend`
- `make preflight-e2e`

If local verification is blocked by environment limits, the blocker will be
reported clearly and remaining checks left to CI.
