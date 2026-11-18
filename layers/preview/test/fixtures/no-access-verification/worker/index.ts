export interface Env { ASSETS: Fetcher; ACCESS_AUD_PREVIEW?: string }

// Trusts that Access is in front. Nothing checks it.
export default {
  async fetch(request, env) {
    return env.ASSETS.fetch(request);
  },
} satisfies ExportedHandler<Env>;
