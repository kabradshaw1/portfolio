import { getDeployInfo, timeAgo } from "@/lib/deployInfo";

export async function EnvironmentChip() {
  const info = await getDeployInfo();

  const commitUrl = info.fullSha
    ? `https://github.com/kabradshaw1/gen_ai_engineer/commit/${info.fullSha}`
    : undefined;

  const age = timeAgo(info.commitDate);

  return (
    <div className="fixed top-16 right-4 z-40 text-xs text-muted-foreground opacity-60 hover:opacity-100 transition-opacity">
      <span>{info.branch}</span>
      <span className="mx-1">·</span>
      {commitUrl ? (
        <a
          href={commitUrl}
          target="_blank"
          rel="noopener noreferrer"
          className="font-mono hover:text-foreground transition-colors"
        >
          {info.commitSha}
        </a>
      ) : (
        <span className="font-mono">{info.commitSha}</span>
      )}
      {age && (
        <>
          <span className="mx-1">·</span>
          <span>{age}</span>
        </>
      )}
    </div>
  );
}
