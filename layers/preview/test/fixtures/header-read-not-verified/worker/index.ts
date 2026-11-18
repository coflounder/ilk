export interface Env { ASSETS: Fetcher; ACCESS_AUD_PREVIEW?: string }

// Present is not the same as genuine: this only checks the header exists.
export default {
  async fetch(request, env) {
    if (!request.headers.get("cf-access-jwt-assertion")) return new Response("no", { status: 401 });
    return env.ASSETS.fetch(request);
  },
} satisfies ExportedHandler<Env>;
