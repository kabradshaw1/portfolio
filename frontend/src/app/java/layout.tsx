import { AuthProvider } from "@/components/java/AuthProvider";
import { SiteHeader } from "@/components/java/SiteHeader";

export default function JavaLayout({ children }: { children: React.ReactNode }) {
  return (
    <AuthProvider>
      <SiteHeader />
      <main className="flex-1">{children}</main>
    </AuthProvider>
  );
}
