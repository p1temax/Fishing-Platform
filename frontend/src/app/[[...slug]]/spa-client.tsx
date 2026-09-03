"use client";

import dynamic from "next/dynamic";

const SpaApp = dynamic(() => import("@/spa/App"), { ssr: false });

export default function SpaClient() {
  return <SpaApp />;
}
