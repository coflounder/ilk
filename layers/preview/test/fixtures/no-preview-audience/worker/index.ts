import { createRemoteJWKSet, jwtVerify } from "jose";

export interface Env {
  ASSETS: Fetcher;
  ACCESS_TEAM_DOMAIN: string;
  ACCESS_AUD: string;
}

// Access already blocks what it does not let in; this guards requests that reach
// the Worker another way.
async function verified(request: Request, env: Env): Promise<boolean> {
  const token = request.headers.get("cf-access-jwt-assertion");
  if (!token) return false;
  const issuer = `https://${env.ACCESS_TEAM_DOMAIN}`;
  const keys = createRemoteJWKSet(new URL(`${issuer}/cdn-cgi/access/certs`));
  const audience = [env.ACCESS_AUD].filter(Boolean);
  try {
    await jwtVerify(token, keys, { issuer, audience });
    return true;
  } catch {
    return false;
  }
}

export default {
  async fetch(request, env) {
    if (!(await verified(request, env))) return new Response("sign in required", { status: 401 });
    return env.ASSETS.fetch(request);
  },
} satisfies ExportedHandler<Env>;

