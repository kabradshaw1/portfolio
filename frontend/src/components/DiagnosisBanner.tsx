interface DiagnosisBannerProps {
  content: string;
}

export function DiagnosisBanner({ content }: DiagnosisBannerProps) {
  return (
    <div className="rounded-lg border border-green-500/30 bg-green-500/10 p-4">
      <h3 className="mb-2 text-sm font-semibold text-green-500 uppercase tracking-wide">
        Diagnosis
      </h3>
      <p className="whitespace-pre-wrap text-sm leading-relaxed">{content}</p>
    </div>
  );
}
