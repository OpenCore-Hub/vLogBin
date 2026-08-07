import { readFile } from "node:fs/promises";

async function readJson(path) {
  return JSON.parse(await readFile(path, "utf8"));
}

async function probeSdk() {
  const client = await readJson("./node_modules/@zitadel/client/package.json");
  const proto = await readJson("./node_modules/@zitadel/proto/package.json");
  const userServiceTypes = await readFile(
    "./node_modules/@zitadel/proto/types/zitadel/user/v2/user_service_pb.d.ts",
    "utf8",
  );

  const hasTypedCreateUser = userServiceTypes.includes("userAction") &&
    userServiceTypes.includes("createUser");

  let latest;
  try {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), 8_000);
    const [clientRes, protoRes] = await Promise.all([
      fetch("https://registry.npmjs.org/@zitadel/client/latest", {
        signal: controller.signal,
      }),
      fetch("https://registry.npmjs.org/@zitadel/proto/latest", {
        signal: controller.signal,
      }),
    ]);
    clearTimeout(timer);
    latest = {
      client: (await clientRes.json()).version,
      proto: (await protoRes.json()).version,
    };
  } catch {
    latest = { client: "unknown", proto: "unknown" };
  }

  const compatShims = [];
  if (!hasTypedCreateUser) {
    compatShims.push("decode create_user via unknown field 6");
    compatShims.push("encode top-level CreateUser metadata via unknown field 6");
  }

  return {
    ok: true,
    installed: { client: client.version, proto: proto.version },
    latest,
    hasTypedCreateUser,
    activeCompatShims: compatShims,
    recommendation: hasTypedCreateUser
      ? "SDK exposes create_user; remove compat shims after real E2E validation"
      : "SDK lags ZITADEL main; keep compat shims and re-run after every SDK upgrade",
  };
}

console.log(JSON.stringify(await probeSdk(), null, 2));
