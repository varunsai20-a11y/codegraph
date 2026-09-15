import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "CodeGraph — Visual Reverse Engineering Interface",
  description: "Google Maps for Software Architecture",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en">
      <body className="antialiased bg-background text-foreground h-screen flex flex-col">
        {children}
      </body>
    </html>
  );
}
