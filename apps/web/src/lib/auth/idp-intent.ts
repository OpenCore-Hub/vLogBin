import { fromBinary, type UnknownField } from "@bufbuild/protobuf";
import { MetadataSchema } from "@zitadel/proto/zitadel/metadata/v2/metadata_pb";
import {
  CreateUserRequestSchema,
  type CreateUserRequest,
} from "@zitadel/proto/zitadel/user/v2/user_service_pb";

export interface IdpCreateUserData {
  username?: string;
  profile?: {
    givenName: string;
    familyName: string;
    displayName?: string;
  };
  email?: { email: string; isVerified: boolean };
  phone?: { phone?: string; isVerified: boolean };
  idpLinks: Array<{ idpId: string; userId: string; userName: string }>;
  metadata: Array<{ key: string; value: Uint8Array }>;
}

interface IntentResponseWithKnownAction {
  userAction?: { case: "createUser"; value: CreateUserRequest } | unknown;
}

interface IntentResponseWithUnknownFields {
  $unknown?: UnknownField[];
}

function lengthDelimitedPayload(data: Uint8Array): Uint8Array | undefined {
  let offset = 0;
  let length = 0;
  let shift = 0;
  while (offset < data.length && shift < 63) {
    const byte = data[offset];
    if (byte === undefined) return undefined;
    offset += 1;
    length |= (byte & 0x7f) << shift;
    if ((byte & 0x80) === 0) {
      if (offset + length > data.length) return undefined;
      return data.subarray(offset, offset + length);
    }
    shift += 7;
  }
  return undefined;
}

function flattenCreateUser(request: CreateUserRequest): IdpCreateUserData {
  const human =
    request.userType?.case === "human" ? request.userType.value : undefined;
  const profile = human?.profile;
  const email = human?.email;
  const phone = human?.phone;
  const topLevelMetadata = (
    request as unknown as IntentResponseWithUnknownFields
  ).$unknown
    ?.filter((field) => field.no === 6 && field.wireType === 2)
    .map((field) => {
      const payload = lengthDelimitedPayload(field.data);
      return payload ? fromBinary(MetadataSchema, payload) : undefined;
    })
    .filter((entry): entry is NonNullable<typeof entry> => Boolean(entry)) ?? [];
  const deprecatedHumanMetadata = human?.metadata ?? [];
  const metadata =
    topLevelMetadata.length > 0
      ? topLevelMetadata.map((entry) => ({
          key: entry.key,
          value: entry.value,
        }))
      : deprecatedHumanMetadata.map((entry) => ({
          key: entry.key,
          value: entry.value,
        }));

  return {
    username: request.username,
    profile: profile
      ? {
          givenName: profile.givenName || "",
          familyName: profile.familyName || "",
          displayName: profile.displayName || undefined,
        }
      : undefined,
    email: email
      ? {
          email: email.email,
          isVerified:
            email.verification?.case === "isVerified" &&
            Boolean(email.verification.value),
        }
      : undefined,
    phone: phone
      ? {
          phone: phone.phone || undefined,
          isVerified:
            phone.verification?.case === "isVerified" &&
            Boolean(phone.verification.value),
        }
      : undefined,
    idpLinks: (human?.idpLinks ?? []).map((link) => ({
      idpId: link.idpId,
      userId: link.userId,
      userName: link.userName,
    })),
    metadata,
  };
}

/**
 * Reads the non-deprecated `user_action.create_user` from an IDP intent
 * response. npm @zitadel/proto 1.3.1 still decodes this field as unknown, so
 * the function first checks the typed field (for SDK upgrades) and then falls
 * back to protobuf unknown-field decoding without guessing byte offsets.
 */
export function decodeIdpCreateUser(
  response: unknown,
): IdpCreateUserData | undefined {
  if (!response) {
    return undefined;
  }
  const known = response as IntentResponseWithKnownAction;
  if (
    known.userAction &&
    typeof known.userAction === "object" &&
    (known.userAction as { case?: unknown }).case === "createUser"
  ) {
    return flattenCreateUser(
      (known.userAction as { value: CreateUserRequest }).value,
    );
  }

  const unknownField = (
    response as IntentResponseWithUnknownFields
  ).$unknown?.find((field) => field.no === 6 && field.wireType === 2);
  if (!unknownField) {
    return undefined;
  }

  try {
    const payload = lengthDelimitedPayload(unknownField.data);
    return payload
      ? flattenCreateUser(fromBinary(CreateUserRequestSchema, payload))
      : undefined;
  } catch {
    return undefined;
  }
}
