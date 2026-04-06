import { GoAuthProvider } from "@/components/go/GoAuthProvider";

export default function GoLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return <GoAuthProvider>{children}</GoAuthProvider>;
}
