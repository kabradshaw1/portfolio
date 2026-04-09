import { GoSubHeader } from "@/components/go/GoSubHeader";

export default function GoEcommerceLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <>
      <GoSubHeader />
      {children}
    </>
  );
}
