# Deploy Beeline for free (Render + Vercel + Neon)

The result is two public links that judges can open:

- **API**: `https://<name>.onrender.com` (Render, free web service, built from `beeline-api/Dockerfile`)
- **Website**: `https://<name>.vercel.app` (Vercel, free, built from `beeline-web/`)

The database is already on Neon (free) and the vault is already on NEAR testnet.

You need free accounts on **render.com** and **vercel.com**. Sign up with GitHub, so both can see the `dev-doji/beeline-relay` repository.

---

## Step 1: The API on Render (about 5 minutes)

1. On render.com, click **New → Blueprint**, pick the `beeline-relay` repository, and confirm. Render reads `render.yaml` and creates the **beeline-api** service on the free plan.
2. Render asks for the secret values. On your PC, in PowerShell from the project folder, run:
   ```powershell
   .\scripts\hosting-env.ps1 -WebUrl https://beeline-relay.vercel.app
   ```
   (Use your expected Vercel address; you can correct it in step 3.) It prints the values **on your screen only**. Copy each one into the matching Render field. Don't paste them anywhere else.
3. Click **Apply**. The first build takes a few minutes. When it shows **Live**, open `https://<your-service>.onrender.com/api/v1/health`. It should say `"status":"ok"`.

## Step 2: The website on Vercel (about 3 minutes)

1. On vercel.com, click **Add New → Project** and import `beeline-relay`.
2. Set **Root Directory** to `beeline-web`. Vercel detects Next.js.
3. Under **Environment Variables**, add:
   - `NEXT_PUBLIC_API_URL` = your Render address, for example `https://beeline-api.onrender.com`
   - `NEXT_PUBLIC_REPO_URL` = `https://github.com/dev-doji/beeline-relay`
4. Click **Deploy**. You get an address like `https://beeline-relay.vercel.app`.

## Step 3: Connect them

If the Vercel address differs from the one you used in step 1: on Render → **beeline-api → Environment**, set `CORS_ALLOWED_ORIGINS` to the exact Vercel address (no trailing slash), then save. Render redeploys automatically.

Open the website, go to **Monitor**, add a request and press **Run monitor**.

---

## Good to know

- **Free-plan sleep:** Render's free services sleep after about 15 minutes without visits. The first visit after that takes about a minute to wake up; after that it's fast. Before judging or recording, open the site once to wake it.
- **Updates:** pushing to `main` redeploys both services automatically.
- **Secrets:** they live only in the Render dashboard and on your PC (`beeline-api/.env`, `beeline-api/.testnet/`). The agent and invoice keys are testnet-only keys.
- **Rate limits:** the public playground allows 4 runs per minute per visitor and 300 per day (`PLAYGROUND_RUNS_PER_*`), because each run uses NEAR AI credits and testnet gas.
