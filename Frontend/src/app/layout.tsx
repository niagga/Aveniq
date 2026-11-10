import type { Metadata } from "next";

import "./globals.css";

export const metadata: Metadata = {
  title: "Aveniq",
  description: "Gestión de consultorios de psicología: pacientes, agenda y facturación.",
};

export default function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="es">
      <body>{children}</body>
    </html>
  );
}
