export type Env = "test" | "live";

export const ENV_COOKIE = "vlb_env";

/** 会话 cookie 名（edge 中间件与服务端共用）。 */
export const SESSION_COOKIE = "vlb_session";

/** First-Run 引导跳过标记（R18：跳过后可随时恢复）。 */
export const ONBOARDING_DISMISS_COOKIE = "vlb_onboarding_dismissed";

/** 当前选中的 workspace（M4 多工作区切换；workspace_id == provider_id）。 */
export const WORKSPACE_COOKIE = "vlb_workspace";

/** 客户门户独立会话 cookie（与平台用户会话隔离）。 */
export const PORTAL_COOKIE = "vlb_portal_session";

/** 会话最长有效期（秒），中间件滑动续期时使用。 */
export const SESSION_MAX_AGE_SECONDS = 60 * 60 * 24 * 7;
