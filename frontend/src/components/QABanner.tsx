import { getDeployInfo } from "@/lib/deployInfo";

export async function QABanner() {
  const info = await getDeployInfo();
  if (!info.isQA) return null;

  return (
    <div className="bg-indigo-600 text-white text-center text-sm py-1.5 px-4">
      You&apos;re viewing the QA environment — latest pre-prod build.
      Production is live at{" "}
      <a
        href="https://kylebradshaw.dev"
        className="underline underline-offset-2 hover:text-indigo-100"
      >
        kylebradshaw.dev
      </a>
      .
    </div>
  );
}
