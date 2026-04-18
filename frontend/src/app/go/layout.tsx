import { GoAuthProvider } from "@/components/go/GoAuthProvider";
import { GoCartProvider } from "@/components/go/GoCartProvider";
import { GoStoreProvider } from "@/components/go/GoStoreProvider";
import { GoSubHeader } from "@/components/go/GoSubHeader";

export default function GoLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <GoAuthProvider>
      <GoCartProvider>
        <GoStoreProvider>
          <GoSubHeader />
          {children}
        </GoStoreProvider>
      </GoCartProvider>
    </GoAuthProvider>
  );
}
