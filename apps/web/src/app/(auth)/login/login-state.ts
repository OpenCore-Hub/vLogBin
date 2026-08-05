/** OIDC 登录流程的 cookie 名称与表单 action 状态类型。
 *  注意：不能放在 "use server" 文件中（Next 禁止其导出非 async 值）。 */
export const OIDC_STATE_COOKIE = "vlb_oidc_state";
export const OIDC_VERIFIER_COOKIE = "vlb_pkce_verifier";
export const OIDC_NEXT_COOKIE = "vlb_oidc_next";

export interface LoginActionState {
  ok: boolean;
  error?: string;
  next?: string;
}
