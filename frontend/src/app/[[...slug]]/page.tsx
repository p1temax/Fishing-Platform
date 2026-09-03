import SpaClient from "./spa-client";

export function generateStaticParams() {
  // Known top-level shells for static export. Deep `/projects/:id/...` links
  // are served via Go NoRoute → index.html + client React Router.
  return [
    { slug: [] },
    { slug: ["login"] },
    { slug: ["workbench"] },
    { slug: ["workbench", "mail"] },
    { slug: ["workbench", "page-builder"] },
    { slug: ["workbench", "info-gathering"] },
    { slug: ["workbench", "qr-phishing"] },
    { slug: ["projects"] },
    { slug: ["agents"] },
    { slug: ["robots"] },
    { slug: ["ip-blacklist"] },
    { slug: ["smtp-services"] },
    { slug: ["audit-logs"] },
    { slug: ["users"] },
    { slug: ["ai-settings"] },
    { slug: ["mail-tracking"] },
  ];
}

export default function SpaCatchAllPage() {
  return <SpaClient />;
}
