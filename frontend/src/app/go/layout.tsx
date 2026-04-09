import { GoAuthProvider } from "@/components/go/GoAuthProvider";
import { GoCartProvider } from "@/components/go/GoCartProvider";
import { GoStoreProvider } from "@/components/go/GoStoreProvider";

export default function GoLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <GoAuthProvider>
      <GoCartProvider>
        <GoStoreProvider>
          {children}
        </GoStoreProvider>
      </GoCartProvider>
    </GoAuthProvider>
  );
}
