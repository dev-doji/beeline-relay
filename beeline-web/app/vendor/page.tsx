import { redirect } from "next/navigation";

// The payment link form became a chat: Bee asks what you need and makes the invoice.
export default function VendorRedirect() {
  redirect("/chat");
}
