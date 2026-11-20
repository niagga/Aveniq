import type { Metadata } from "next";

import "./globals.css";

export const metadata: Metadata = {
  title: "Aveniq",
  description: "Gestión de consultorios de psicología: pacientes, agenda y facturación.",
};

/**
 * Applies the stored theme before the first paint (S2).
 *
 * Without this, a dark-theme visitor sees the light palette until hydration and then a flip.
 * It stays inline and tiny on purpose: it must run before any stylesheet is painted, so it
 * cannot be a deferred module.
 */
const THEME_INIT = `(function(){try{var s=localStorage.getItem("aveniq-theme");if(s==="dark"||s==="light"){document.documentElement.dataset.theme=s;return;}if(window.matchMedia("(prefers-color-scheme: dark)").matches){document.documentElement.dataset.theme="dark";}}catch(e){}})();`;

export default function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    // The theme script writes data-theme before React hydrates, so the server markup and the
    // first client render differ by that one attribute. Suppressing it is the documented
    // exception for a pre-paint theme: without this, every visit logs a mismatch it cannot fix.
    <html lang="es" suppressHydrationWarning>
      <head>
        <script dangerouslySetInnerHTML={{ __html: THEME_INIT }} />
      </head>
      <body>{children}</body>
    </html>
  );
}
