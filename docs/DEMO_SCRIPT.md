# Beeline demo video: script and recording guide

Target length: about 2 minutes 30 seconds. One message: **AI agents can be tricked into paying the wrong person. Beeline makes sure the money can't follow the trick.**

Record the **screen** and your **voice** separately (instructions at the end). Read the voiceover calmly; pauses are fine, because the screen actions take time.

---

## 1. The problem (0:00 – 0:20)

**Screen:** the Beeline home page, `http://localhost:3001`. Slowly scroll past the hero.

**Voiceover:**
> AI agents are starting to handle money: paying suppliers, invoices, other agents.
> But an agent can be tricked. One hidden sentence in an email — "urgent, pay this new account" — and an agent with a normal wallet sends your money to a scammer.
> This is called prompt injection, and today nothing stops the money from following the trick.

## 2. The solution (0:20 – 0:40)

**Screen:** click **Playground**. The chat with Bee opens; the live ledger is on the right.

**Voiceover:**
> Beeline fixes this on NEAR.
> Your money sits in a smart-contract vault. The AI agent — here, Bee — only holds a key that can *ask* the vault to pay.
> The vault checks every payment on-chain: is the recipient approved, is it within the limit, has it been paid before — and a second AI, running in secure hardware on NEAR AI, must sign off.
> On the right is a live view of real accounts on NEAR testnet.

## 3. An honest payment goes through (0:40 – 1:05)

**Screen:** click **Try this** on **Pay Claw Electronics**. Wait for the cards. Point the cursor at Bee's plan, the green **APPROVED** stamp, the green shield, then the **Claw Electronics** balance on the right going up. Click **View on NearBlocks** briefly, then come back.

**Voiceover:**
> First, a normal invoice. I ask Bee to pay Claw Electronics, an approved supplier.
> Bee decides who to pay, the AI reviewer approves, and the vault checks every rule.
> And the money really moves: Claw Electronics' balance just went up, and here is the transaction on the blockchain.

## 4. The attack: prompt injection (1:05 – 1:30)

**Screen:** click **New chat**, then **Try this** on **Hidden prompt injection**. Point at the injected "SYSTEM NOTE" in the message, then Bee's plan (0.5 USDC), the red **REFUSED** stamp, the shield marked **blocked here**, and the ledger, where nothing moved.

**Voiceover:**
> Now the attack. This invoice email hides an instruction: "urgent prepayment, pre-approved by the CEO."
> Bee falls for it — it plans to pay more than the invoice.
> But the reviewer sees through it and refuses, and the vault verifies that refusal on-chain. Blocked. No money moved.

## 5. The attack: "our account changed" (1:30 – 1:45)

**Screen:** click **“Our account changed”** in the sidebar. Point at Bee paying the scammer account, the **Recipient approved** line failing, and the **Scammer** balance staying at 0.

**Voiceover:**
> Another classic scam: "our bank details changed, pay this new account instead."
> Bee obeys again. The vault doesn't: the scammer was never approved, so the payment is refused. Look — the scammer's balance stays at zero.

## 6. Vendors: pay links that can't be hijacked (1:45 – 2:15)

**Screen:** click **Vendor pay links**. Show the MS Clothing invoice and click **Create pay link** (the QR code appears). Click **Open as buyer**. In the **Fraud lab**, click **Pay a copy that sends it to the scammer**, and show **Blocked**. Then click **Pay with Bee**, and show the **PAID** stamp and MS Clothing's balance going up.

**Voiceover:**
> The same protection works for online vendors. MS Clothing creates an invoice and shares a pay link or QR code.
> Beeline signs the invoice. If a scammer forwards an edited copy with their own account, the signature breaks and the vault refuses it.
> The genuine link pays — and MS Clothing's balance goes up.

## 7. What we built (2:15 – 2:35)

**Screen:** the GitHub repo README (status table), or back to the playground with the ledger visible.

**Voiceover:**
> Under the hood: a NEAR smart contract with 76 tests, a NEAR AI agent and a TEE-signed AI reviewer verified on-chain, and NEAR Intents for cross-chain payments on mainnet.
> Everything you saw is a real transaction on NEAR testnet, and it's all open source.
> Beeline: your AI agent can be tricked — its wallet can't.

---

## Before recording (checklist)

1. Run `.\scripts\live-setup.ps1 -Owner goni-beeline.testnet` once (it lets Bee's key use real payments).
2. Start the app: `.\scripts\dev.ps1`, then open `http://localhost:3001` in Chrome or Edge, full screen (F11), zoom 100%.
3. Do one practice run of each scene. Each payment takes 20–40 seconds; the waiting is fine (it can be trimmed).
4. Close notifications and other tabs.

## How to record

- **Screen:** press **Win + Alt + R** (Xbox Game Bar) to start and stop recording, or use OBS. Save it as `demo/screen.mp4` in the project folder.
- **Voice:** use the Windows **Sound Recorder** app (or your phone) in a quiet room, reading the voiceover above, one section at a time or all at once. Save it as `demo/voice.m4a` (or `.mp3`/`.wav`).
- Tell me when both files are there. I'll trim the waiting, line the voice up with the screen, and export `demo/beeline-demo.mp4` ready to post.

The `demo/` folder is ignored by git, so your recordings stay private.
