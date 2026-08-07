import { cookies, headers } from "next/headers";
import { NextResponse, type NextRequest } from "next/server";
import { recordAuthEvent } from "@/lib/auth/auth-events";
import {
  isCustomLoginAllowedForOrg,
  isCustomLoginAllowedForUser,
} from "@/lib/auth/config";
import { getOrCreateDeviceFingerprint } from "@/lib/auth/device";
import {
  clearIdpFlow,
  getIdpFlow,
  setIdpFlow,
  setLoginFlow,
} from "@/lib/auth/login-flow";
import { rememberSession } from "@/lib/auth/zitadel-sessions-store";
import {
  createCallback,
  createSession,
  createUser,
  getActiveIdentityProviders,
  getSession,
  getUserByID,
  retrieveIdpIntent,
  validateSession,
} from "@/lib/auth/zitadel-session";
import { OIDC_NEXT_COOKIE } from "../../login/login-state";

function fail(
  req: NextRequest,
  error: string,
  description?: string,
): NextResponse {
  const url = new URL("/idps/failure", req.url);
  url.searchParams.set("error", error);
  if (description) {
    url.searchParams.set("description", description);
  }
  return NextResponse.redirect(url);
}

async function completeIdpLogin(req: NextRequest, input: {
  sessionId: string;
  sessionToken: string;
  loginName: string;
  userId: string;
  organizationId?: string;
  authRequestId?: string;
  next?: string;
}): Promise<NextResponse> {
  if (!input.authRequestId) {
    return fail(req, "missing_auth_request", "登录请求已失效，请重新开始");
  }
  const callback = await createCallback({
    authRequestId: input.authRequestId,
    sessionId: input.sessionId,
    sessionToken: input.sessionToken,
  });
  recordAuthEvent("custom_login.callback.completed", {
    userId: input.userId,
    organizationId: input.organizationId,
  });
  const jar = await cookies();
  if (input.next) {
    jar.set(OIDC_NEXT_COOKIE, input.next, {
      httpOnly: true,
      secure: process.env.NODE_ENV === "production",
      sameSite: "lax",
      path: "/",
      maxAge: 600,
    });
  }
  await rememberSession({
    sessionId: input.sessionId,
    sessionToken: input.sessionToken,
    loginName: input.loginName,
    userId: input.userId,
    organizationId: input.organizationId,
  });
  await clearIdpFlow();
  return NextResponse.redirect(new URL(callback.callbackUrl, req.url));
}

export async function GET(req: NextRequest) {
  const { searchParams } = req.nextUrl;
  const idpIntentId = searchParams.get("id") ?? searchParams.get("idpIntentId");
  const idpIntentToken =
    searchParams.get("token") ?? searchParams.get("idpIntentToken");
  const oidcError = searchParams.get("error");
  const flow = await getIdpFlow();

  if (!flow?.idpId || !idpIntentId || !idpIntentToken) {
    return fail(req, "idp_state_expired", "企业身份登录状态已失效，请重新开始");
  }
  if (oidcError) {
    recordAuthEvent("custom_login.idp.intent.failed", {
      idpId: flow.idpId,
      error: oidcError.slice(0, 200),
    });
    return fail(req, "idp_denied", oidcError);
  }

  try {
    const intent = await retrieveIdpIntent({
      idpIntentId,
      idpIntentToken,
    });
    recordAuthEvent("custom_login.idp.intent.success", {
      idpId: intent.idpId,
    });
    const providers = await getActiveIdentityProviders();
    const provider = providers.find((candidate) => candidate.id === intent.idpId);
    if (!provider) {
      return fail(req, "idp_unavailable", "企业身份源当前不可用");
    }

    let userId = intent.userId;
    if (!userId && intent.addHumanUser) {
      const add = intent.addHumanUser;
      const hasCompleteProfile = Boolean(
        add.profile?.givenName &&
          add.profile?.familyName &&
          add.email?.email,
      );
      if (provider.isAutoCreation && hasCompleteProfile) {
        const created = await createUser({
          email: add.email?.email ?? "",
          givenName: add.profile?.givenName ?? "",
          familyName: add.profile?.familyName ?? "",
          sendEmailCode: !add.email?.isVerified,
          username: add.username,
          idpLinks:
            add.idpLinks.length > 0
              ? add.idpLinks
              : [
                  {
                    idpId: intent.idpId,
                    userId: intent.idpUserId,
                    userName: intent.idpUserName,
                  },
                ],
          metadata: add.metadata,
        });
        userId = created.userId;
        recordAuthEvent("custom_login.idp.auto_created", {
          userId: created.userId,
        });
      } else if (provider.isCreationAllowed) {
        await setIdpFlow({
          ...flow,
          idpIntentId,
          idpIntentToken,
          idpUserId: intent.idpUserId,
          idpUserName: intent.idpUserName,
          givenName: add.profile?.givenName,
          familyName: add.profile?.familyName,
          email: add.email?.email,
          emailVerified: add.email?.isVerified,
          metadata: add.metadata.map((entry) => ({
            key: entry.key,
            valueBase64: Buffer.from(entry.value).toString("base64"),
          })),
        });
        recordAuthEvent("custom_login.idp.registration_required", {
          idpId: intent.idpId,
        });
        return NextResponse.redirect(
          new URL("/idps/complete-registration", req.url),
        );
      }
    }

    if (!userId) {
      recordAuthEvent("custom_login.idp.intent.failed", {
        idpId: intent.idpId,
        error: "account_not_linked",
      });
      return fail(req, "account_not_linked", "该企业账号尚未关联 vLogBin 用户");
    }

    const user = await getUserByID(userId);
    if (
      !isCustomLoginAllowedForUser(userId) ||
      !isCustomLoginAllowedForOrg(user.organizationId)
    ) {
      return fail(req, "not_in_custom_login_scope", "该账号不在自建登录范围");
    }

    const requestHeaders = await headers();
    const userAgent = requestHeaders.get("user-agent") ?? "";
    const ip =
      requestHeaders.get("x-forwarded-for")?.split(",")[0]?.trim() ||
      requestHeaders.get("x-real-ip") ||
      "0.0.0.0";
    const fingerprintId = await getOrCreateDeviceFingerprint();
    const created = await createSession({
      userId,
      idpIntent: { idpIntentId, idpIntentToken },
      fingerprintId,
      ip,
      description: userAgent.slice(0, 200),
      userAgentHeader: userAgent
        ? { "user-agent": [userAgent] }
        : undefined,
      lifetimeSeconds: 24 * 60 * 60,
    });
    const session = await getSession({
      sessionId: created.sessionId,
      sessionToken: created.sessionToken,
    });
    const validation = await validateSession(session);
    const loginFlow = {
      sessionId: created.sessionId,
      sessionToken: created.sessionToken,
      loginName: session.user?.loginName || user.preferredLoginName || user.username,
      userId,
      organizationId: session.user?.organizationId || user.organizationId,
      authRequestId: flow.authRequestId,
      next: flow.next,
    };
    if (validation.valid) {
      return await completeIdpLogin(req, loginFlow);
    }
    if (validation.reason === "mfa-required" && session.user) {
      await clearIdpFlow();
      await setLoginFlow(loginFlow);
      recordAuthEvent("custom_login.idp.mfa_required", {
        userId: session.user.id,
      });
      return NextResponse.redirect(
        new URL(
          `/login?authRequest=${encodeURIComponent(flow.authRequestId ?? "")}&idpMfa=1`,
          req.url,
        ),
      );
    }
    return fail(
      req,
      "session_incomplete",
      validation.reason === "email-not-verified"
        ? "请先完成邮箱验证"
        : "登录尚未完成，请重新开始",
    );
  } catch (err) {
    recordAuthEvent("custom_login.idp.intent.failed", {
      idpId: flow.idpId,
      error: err instanceof Error ? err.message.slice(0, 200) : "unknown",
    });
    return fail(
      req,
      "idp_processing_failed",
      err instanceof Error ? err.message : "企业身份登录处理失败",
    );
  }
}
