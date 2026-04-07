import { GoAuthProvider } from "@/components/go/GoAuthProvider";
import { GoCartProvider } from "@/components/go/GoCartProvider";
import { GoSubHeader } from "@/components/go/GoSubHeader";

export default function GoLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <GoAuthProvider>
      <GoCartProvider>
        <GoSubHeader />
        {children}
      </GoCartProvider>
    </GoAuthProvider>
  );
}
