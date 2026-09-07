// 根布局（M19）：注入 session → 顶栏 → 页面。
import type { Metadata } from "next";
import { auth } from "@/auth";
import { Navbar } from "@/components/Navbar";
import "./globals.css";

export const metadata: Metadata = {
  title: "JSL 租户门户",
  description: "JSL AIoT 平台租户门户骨架（M19）",
};

export default async function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const session = await auth();
  return (
    <html lang="zh-CN">
      <body>
        {session?.accessToken && (
          <Navbar
            subject={session.subject}
            tenantId={session.tenantId}
            roles={session.roles}
          />
        )}
        <main className="mx-auto max-w-6xl px-6 py-8">{children}</main>
      </body>
    </html>
  );
}
