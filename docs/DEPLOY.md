# Deploy Beeline for free (Render + Vercel + Neon)

The result is two public links that judges can open:

- **API**: `https://<name>.onrender.com` (Render, free web service, built from `beeline-api/Dockerfile`)
- **Website**: `https://usebeeline.xyz` (Vercel, free, built from `beeline-web/`, with our own domain)

The database is already on Neon (free) and the vault is already on NEAR testnet.

You need free accounts on **render.com** and **vercel.com**. Sign up with GitHub, so both can see the `dev-doji/beeline-relay` repository.

---

## Step 1: The API on Render (about 5 minutes)

1. On render.com, click **New → Blueprint**, pick the `beeline-relay` repository, and confirm. Render reads `render.yaml` and creates the **beeline-api** service on the free plan.
2. Render asks for the secret values. On your PC, in PowerShell from the project folder, run:
   ```powershell
   .\scripts\hosting-env.ps1
   ```
   By default it allows `https://usebeeline.xyz` and `https://www.usebeeline.xyz` to call the API. To allow other addresses as well, list them all: `-WebUrl https://usebeeline.xyz, https://<name>.vercel.app`. It prints the values **on your screen only**. Copy each one into the matching Render field. Don't paste them anywhere else.
3. Click **Apply**. The first build takes a few minutes. When it shows **Live**, open `https://<your-service>.onrender.com/api/v1/health`. It should say `"status":"ok"`.

## Step 2: The website on Vercel (about 3 minutes)

1. On vercel.com, click **Add New → Project** and import `beeline-relay`.
2. Set **Root Directory** to `beeline-web`. Vercel detects Next.js.
3. Under **Environment Variables**, add:
   - `NEXT_PUBLIC_API_URL` = your Render address, for example `https://beeline-api.onrender.com`
   - `NEXT_PUBLIC_REPO_URL` = `https://github.com/dev-doji/beeline-relay`
4. Click **Deploy**. You get an address like `https://beeline-relay.vercel.app`.
5. Under **Settings → Domains**, add `usebeeline.xyz` (Vercel offers to add `www.usebeeline.xyz` too). Vercel shows one or two DNS records; add them where you bought the domain. The domain starts working once DNS updates, usually within minutes, sometimes up to a few hours.

## Step 3: Connect them

The API only answers the website addresses in `CORS_ALLOWED_ORIGINS` (comma-separated, no trailing slash). To use another address, for example the `.vercel.app` one, go to Render → **beeline-api → Environment** and add it to the list, then save. Render redeploys automatically.

Open the website, go to **Monitor**, add a request and press **Run monitor**.

---

## Optional: switch on email wallets (Privy)

This turns on "Sign in with email" and an automatic Base wallet on the `/fund` page. Without it, the page says the feature is not switched on, and nothing else changes.

1. Sign up at **dashboard.privy.io** (the free plan covers up to 499 monthly active users) and create an app for the **web**.
2. Copy the **App ID**. It is public, not a secret.
3. In the app's settings:
   - **Login methods:** enable **Email** only.
   - **Embedded wallets:** enable **Ethereum**, created automatically on login.
   - **Allowed origins / domains:** add `https://usebeeline.xyz` and `https://www.usebeeline.xyz` (and `http://localhost:3001` for local testing). If this is missing, the page shows "Sign-in could not start".
4. For free-of-ETH sends, open **Fee sponsorship**: turn on **Sponsor gas fees**, add **Base**, and make sure the app uses **TEE execution** (Privy requires it for sponsorship). Prepaid accounts need a saved payment method and bought credits (Privy publishes no rate). A few dollars of credits is plenty for testing. Without this, sends fail and nothing moves, and the wallet can still receive USDC.
5. In **Vercel** (project settings, Environment Variables), add `NEXT_PUBLIC_PRIVY_APP_ID` with the App ID, then redeploy.

## Good to know

- **Free-plan sleep:** Render's free services sleep after about 15 minutes without visits. The first visit after that takes about a minute to wake up; after that it's fast. Before judging or recording, open the site once to wake it.
- **Updates:** pushing to `main` redeploys both services automatically.
- **Secrets:** they live only in the Render dashboard and on your PC (`beeline-api/.env`, `beeline-api/.testnet/`). The agent and invoice keys are testnet-only keys.
- **Rate limits:** the public playground allows 4 runs per minute per visitor and 300 per day (`PLAYGROUND_RUNS_PER_*`), because each run uses NEAR AI credits and testnet gas.
