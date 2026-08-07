import { fromBinary, toBinary } from "@bufbuild/protobuf";
import { create } from "@zitadel/client";
import { MetadataSchema } from "@zitadel/proto/zitadel/metadata/v2/metadata_pb";
import {
  CreateUserRequestSchema,
  RetrieveIdentityProviderIntentResponseSchema,
} from "@zitadel/proto/zitadel/user/v2/user_service_pb";
import { describe, expect, it } from "vitest";
import { decodeIdpCreateUser } from "./idp-intent";

function concatBytes(parts: Uint8Array[]): Uint8Array {
  const length = parts.reduce((total, part) => total + part.length, 0);
  const result = new Uint8Array(length);
  let offset = 0;
  for (const part of parts) {
    result.set(part, offset);
    offset += part.length;
  }
  return result;
}

function varint(value: number): Uint8Array {
  const bytes: number[] = [];
  let remaining = value;
  while (remaining > 0x7f) {
    bytes.push((remaining & 0x7f) | 0x80);
    remaining = Math.floor(remaining / 0x80);
  }
  bytes.push(remaining);
  return Uint8Array.from(bytes);
}

function lengthDelimitedField(fieldNo: number, data: Uint8Array): Uint8Array {
  const tag = varint((fieldNo << 3) | 2);
  return concatBytes([tag, varint(data.length), data]);
}

function responseWithCreateUser(createUserData: Uint8Array): unknown {
  return fromBinary(
    RetrieveIdentityProviderIntentResponseSchema,
    lengthDelimitedField(6, createUserData),
  );
}

function createUserRequest() {
  return create(CreateUserRequestSchema, {
    username: "idp-user@example.com",
    userType: {
      case: "human",
      value: {
        profile: {
          givenName: "Kilgore",
          familyName: "Trout",
          displayName: "Kilgore Trout",
        },
        email: {
          email: "idp-user@example.com",
          verification: { case: "isVerified", value: true },
        },
        idpLinks: [
          {
            idpId: "idp-1",
            userId: "external-user-1",
            userName: "kilgore",
          },
        ],
        metadata: [{ key: "tenant", value: new Uint8Array([1, 2, 3]) }],
      },
    },
  });
}

describe("decodeIdpCreateUser", () => {
  it("decodes the typed user_action.create_user field", () => {
    const data = decodeIdpCreateUser({
      userAction: { case: "createUser", value: createUserRequest() },
    });

    expect(data?.username).toBe("idp-user@example.com");
    expect(data?.profile).toEqual({
      givenName: "Kilgore",
      familyName: "Trout",
      displayName: "Kilgore Trout",
    });
    expect(data?.email).toEqual({
      email: "idp-user@example.com",
      isVerified: true,
    });
    expect(data?.idpLinks).toEqual([
      {
        idpId: "idp-1",
        userId: "external-user-1",
        userName: "kilgore",
      },
    ]);
    expect(data?.metadata).toEqual([
      { key: "tenant", value: new Uint8Array([1, 2, 3]) },
    ]);
  });

  it("decodes create_user from unknown field 6", () => {
    const data = decodeIdpCreateUser(
      responseWithCreateUser(
        toBinary(CreateUserRequestSchema, createUserRequest()),
      ),
    );

    expect(data?.username).toBe("idp-user@example.com");
    expect(data?.profile?.givenName).toBe("Kilgore");
    expect(data?.email?.isVerified).toBe(true);
  });

  it("decodes top-level metadata from the create_user unknown field", () => {
    const request = create(CreateUserRequestSchema, {
      username: "idp-user@example.com",
      userType: {
        case: "human",
        value: {
          profile: { givenName: "Kilgore", familyName: "Trout" },
          email: {
            email: "idp-user@example.com",
            verification: { case: "isVerified", value: true },
          },
        },
      },
    });
    const metadata = toBinary(
      MetadataSchema,
      create(MetadataSchema, {
        key: "tenant",
        value: new Uint8Array([1, 2, 3]),
      }),
    );
    const requestWithUnknownMetadata = concatBytes([
      toBinary(CreateUserRequestSchema, request),
      lengthDelimitedField(6, metadata),
    ]);
    const data = decodeIdpCreateUser(
      responseWithCreateUser(requestWithUnknownMetadata),
    );

    expect(data?.metadata).toEqual([
      { key: "tenant", value: new Uint8Array([1, 2, 3]) },
    ]);
  });

  it("returns undefined without create_user information", () => {
    expect(decodeIdpCreateUser({})).toBeUndefined();
    expect(decodeIdpCreateUser(null)).toBeUndefined();
  });
});
