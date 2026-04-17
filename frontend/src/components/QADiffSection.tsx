import { getDeployInfo, timeAgo, REPO } from "@/lib/deployInfo";

export async function QADiffSection() {
  const info = await getDeployInfo();
  const commits = info.qaAheadOfMain;

  return (
    <div className="mt-8">
      <h3 className="text-lg font-semibold">
        What&apos;s currently staged on QA
      </h3>
      {commits.length === 0 ? (
        <p className="mt-2 text-sm text-muted-foreground">
          QA is caught up with production — latest work is live.
        </p>
      ) : (
        <>
          <p className="mt-2 text-sm text-muted-foreground">
            {commits.length} commit{commits.length !== 1 ? "s" : ""} on{" "}
            <code>qa</code> not yet on <code>main</code>:
          </p>
          <div className="mt-3 space-y-2">
            {commits.map((c) => (
              <div
                key={c.sha}
                className="flex items-baseline gap-3 text-sm"
              >
                <a
                  href={c.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="font-mono text-xs text-muted-foreground hover:text-foreground transition-colors shrink-0"
                >
                  {c.sha}
                </a>
                <span className="text-foreground truncate">{c.message}</span>
                <span className="text-xs text-muted-foreground shrink-0">
                  {timeAgo(c.date)}
                </span>
              </div>
            ))}
          </div>
          {commits.length >= 20 && (
            <a
              href={`https://github.com/${REPO}/compare/main...qa`}
              target="_blank"
              rel="noopener noreferrer"
              className="mt-3 inline-block text-sm text-muted-foreground underline underline-offset-2 hover:text-foreground"
            >
              View all on GitHub →
            </a>
          )}
        </>
      )}
    </div>
  );
}
