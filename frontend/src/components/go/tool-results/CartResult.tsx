import { type CartItem } from "./types";

export function CartResult({ cart: _cart }: { cart: { items: CartItem[]; total: number } }) {
  return null;
}
