import Link from "next/link";

interface ProductCardProps {
  id: string;
  name: string;
  category: string;
  priceCents: number;
  imageUrl?: string;
}

function formatPrice(cents: number): string {
  return `$${(cents / 100).toFixed(2)}`;
}

export function ProductCard({ id, name, category, priceCents, imageUrl }: ProductCardProps) {
  return (
    <Link
      href={`/go/ecommerce/${id}`}
      className="group block rounded-lg border border-foreground/10 p-4 transition-colors hover:border-foreground/20"
    >
      <div className="aspect-square rounded-md bg-muted flex items-center justify-center overflow-hidden">
        {imageUrl ? (
          <img src={imageUrl} alt={name} className="size-full object-cover" />
        ) : (
          <span className="text-3xl text-muted-foreground/40">&#128722;</span>
        )}
      </div>
      <p className="mt-3 text-xs uppercase tracking-wider text-muted-foreground">
        {category}
      </p>
      <h3 className="mt-1 text-sm font-medium group-hover:text-primary transition-colors">
        {name}
      </h3>
      <p className="mt-1 text-sm font-semibold">{formatPrice(priceCents)}</p>
    </Link>
  );
}
