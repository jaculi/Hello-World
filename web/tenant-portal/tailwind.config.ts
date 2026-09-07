import type { Config } from "tailwindcss";

const config: Config = {
  content: [
    "./app/**/*.{ts,tsx}",
    "./components/**/*.{ts,tsx}",
    "./lib/**/*.{ts,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        // 平台主题（骨架临时值；正式主题随 BC-P4 i18n + 设计系统落地）
        brand: {
          50: "#eef2ff",
          500: "#4f46e5",
          600: "#4338ca",
          700: "#3730a3",
        },
      },
    },
  },
  plugins: [],
};

export default config;
