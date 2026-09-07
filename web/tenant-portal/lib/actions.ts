// 服务端 actions（M19）：包装 Auth.js signIn/signOut 为 form action 兼容签名。
// 原始 signIn/signOut 签名带 provider 泛型参数，与 <form action={fn}> 期望的
// (formData: FormData) => Promise<void> 不兼容，故包一层。
"use server";

import { signIn, signOut } from "@/auth";

export async function loginWithKeycloak() {
  await signIn("keycloak", { redirectTo: "/" });
}

export async function logout() {
  await signOut({ redirectTo: "/login" });
}
