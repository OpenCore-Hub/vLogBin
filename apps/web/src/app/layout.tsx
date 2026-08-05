import type { Metadata } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import "./globals.css";
import { ToastProvider } from "@/components/ui/toast";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  title: {
    default: "vLogBin — 计费即代码",
    template: "%s · vLogBin",
  },
  description:
    "vLogBin 以账单即代码为理念，为云原生服务提供用量计量、套餐目录与订阅计费的一体化基础设施。",
};

/**
 * 主题防闪烁：在首帧前根据 cookie 偏好（vlb_theme）设置 <html class="dark">，
 * 避免暗色主题用户在刷新时看到亮色闪白。
 */
const themeInitScript = `(function(){try{var m=document.cookie.match(/(?:^|; )vlb_theme=([^;]*)/);var t=m&&m[1]?m[1]:'';if(t==='dark'||(t!=='light'&&window.matchMedia('(prefers-color-scheme: dark)').matches)){document.documentElement.classList.add('dark')}}catch(e){}})();`;

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html
      lang="zh-CN"
      className={`${geistSans.variable} ${geistMono.variable} h-full antialiased`}
    >
      <body className="min-h-dvh bg-canvas text-foreground">
        <script dangerouslySetInnerHTML={{ __html: themeInitScript }} />
        <ToastProvider>{children}</ToastProvider>
      </body>
    </html>
  );
}
