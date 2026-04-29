<!-- BEGIN:nextjs-agent-rules -->
# This is NOT the Next.js you know

This version has breaking changes — APIs, conventions, and file structure may all differ from your training data. Read the relevant guide in `node_modules/next/dist/docs/` before writing any code. Heed deprecation notices.
<!-- END:nextjs-agent-rules -->

Critical deployment rule: if frontend code adds a new `NEXT_PUBLIC_*` env var
with a `localhost` fallback, add the variable in Vercel and trigger a redeploy
before merging. Otherwise Vercel can bake the localhost fallback into the
production bundle.
