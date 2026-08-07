"use client";

import { useActionState } from "react";
import { completeIdpRegistration } from "../../login/custom-login-actions";
import { CustomLoginActionState } from "../../login/login-state";
import { Button } from "@/components/ui/button";
import { Field, Input } from "@/components/ui/field";
import { Alert } from "@/components/ui/feedback";
import { UsersIcon } from "@/components/ui/icons";

const initialState: CustomLoginActionState = {
  ok: false,
  step: "identifier",
};

export function IdpRegistrationForm({
  givenName,
  familyName,
  email,
}: {
  givenName: string;
  familyName: string;
  email: string;
}) {
  const [state, action, pending] = useActionState(
    completeIdpRegistration,
    initialState,
  );
  return (
    <form action={action} className="space-y-4">
      <Field label="邮箱" htmlFor="idp-registration-email" hint="邮箱由企业身份源确认，不可修改">
        <Input
          id="idp-registration-email"
          name="email"
          value={email}
          readOnly
          autoComplete="email"
          className="bg-surface-2"
        />
      </Field>
      <div className="grid grid-cols-2 gap-3">
        <Field label="名字" htmlFor="idp-registration-given-name">
          <Input
            id="idp-registration-given-name"
            name="givenName"
            defaultValue={givenName}
            autoComplete="given-name"
            required
            maxLength={200}
          />
        </Field>
        <Field label="姓氏" htmlFor="idp-registration-family-name">
          <Input
            id="idp-registration-family-name"
            name="familyName"
            defaultValue={familyName}
            autoComplete="family-name"
            required
            maxLength={200}
          />
        </Field>
      </div>
      {state.error && <Alert variant="danger">{state.error}</Alert>}
      <Button type="submit" size="lg" loading={pending} className="w-full">
        <UsersIcon size={15} />
        创建账号
      </Button>
    </form>
  );
}
