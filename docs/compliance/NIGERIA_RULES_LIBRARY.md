# Nigeria Preset Rules Library (source of truth)

> Saved verbatim from the product owner's brief (Prompt 3, Appendix) on 2026-10-02. This file is the
> source of truth for the Nigeria rule pack (`backend/internal/rules/packs/ng/`): seed data, the
> completeness tests and the dashboard's plain-English text are checked against it. Change it only
> with the compliance lead, and update the seed in the same commit.

## Nigeria Preset Rules: Transaction Monitoring and Cybersecurity

2026-10-02 · Justus

### 1. How to read this library

This is a starter library of 71 preset rules and 12 governance presets for a Nigerian risk engine, drawn from the CBN, NIBSS, NFIU, NCC, SEC and FCCPC, described in plain English in section 7, with the engineering IF / WHEN / THEN logic in section 7B. Only the 19 regulatory controls (REG-01 to REG-19) come directly from law or regulator directives; the rest are recommended defaults you must tune on real data.

| Status | Meaning | Where it appears |
|---|---|---|
| Regulatory | Required by a Nigerian law or CBN circular. Encode as a hard control, not a weighted score | Section 2 |
| Recommended | Common fraud and money-laundering typology. Starting weights and thresholds are my defaults, not regulation, and must be tuned per customer | Sections 3 and 4 |
| Governance | Process requirements that shape alerts, cases, reporting and model oversight | Section 5 |

**Three things to know before you encode anything**

1. **Primary documents were not all readable.** The CBN website blocked direct downloads, so most CBN figures come from law-firm and press summaries of the circulars. Verify each one against the circular before it ships; section 9 lists which ones to double-check.
2. **Recent changes matter most.** The CBN instant-payment guidelines took effect on 1 July 2026, the CBN automated AML baseline standards were issued on 10 March 2026, and the weekly cash limits changed on 1 January 2026. Older tier-limit figures found online are outdated.
3. **Amounts are in naira and must be configurable.** Every threshold below is a default a tenant can change, and every rule should store its legal source and effective date so a regulator or customer can see why it exists.

This library is research support, not legal advice. Have Nigerian counsel confirm the regulatory rows before launch.

### 2. Regulatory hard controls

Nineteen controls come straight from Nigerian law or regulator directives (14 in the first table, 5 in the second), and they should be hard limits or mandatory workflows rather than weighted signals; source numbers refer to the list in section 9.

| ID | Control | Trigger | Required action | Basis |
|---|---|---|---|---|
| REG-01 | New-device cap | Mobile app device bound under 24 hours ago, or account opened under 24 hours ago, and amount above ₦20,000 | Decline; institutions may set a lower cap | CBN instant payment guidelines, in force 1 Jul 2026 (S2) |
| REG-02 | Single active device | Login on a second device while another is bound to the same mobile banking app | Block the second session; re-activation with fresh authentication on the new device | S2 |
| REG-03 | New-device web login | First internet-banking login from an unrecognized device | Enhanced multi-factor authentication | S2 |
| REG-04 | High-value ceiling | Transfer above ₦25m (individual) or ₦250m (corporate), unless the limit was raised after enhanced due diligence and multi-factor authentication | Block or route to EDD | S2 |
| REG-05 | Suspect-BVN hold | BVN on a temporary suspicious watchlist | Hold the transfer up to 24 hours while investigating | S2 |
| REG-06 | Digital onboarding identity | Digital account opening or reactivation without a liveness check or a live BVN or NIN verification | Block the account action | S2 |
| REG-07 | Cash payment ceiling | Cash payment above ₦5m (individual) or ₦10m (corporate body) | Flag; amounts above the ceiling must go through a financial institution | Money Laundering (Prevention and Prohibition) Act 2022 (S4) |
| REG-08 | Weekly cash withdrawal limit | Cash withdrawals across all channels above ₦1.5m (individual) or ₦5m (corporate) in a week | Charge 3% (individual) or 5% (corporate) on the excess; applies to banks, so make it a tenant toggle | CBN cash policy, effective 1 Jan 2026 (S3) |
| REG-09 | ATM limit | ATM withdrawals above ₦100,000 per day or ₦850,000 per week per customer | Decline | S3 |
| REG-10 | Cross-border report | International transfer of US$10,000 or more (or equivalent) | Generate a report to the NFIU within one day of the transaction | Act 2022 as summarized in S4 |
| REG-11 | Sanctions match | Name, ID or account match to the Nigeria sanctions list, UN lists or other configured lists, using fuzzy and transliteration-aware matching | Block, freeze, alert and report to the NFIU and Nigeria Sanctions Committee | Terrorism (Prevention and Prohibition) Act 2022 (S8); CBN baseline standards 5.3 (S1) |
| REG-12 | Politically exposed person | Customer or beneficial owner is a PEP | Enhanced due diligence, source of funds and wealth, senior management approval | Act 2022 (S4); S1 |
| REG-13 | Person-of-interest match | Customer or counterparty matches the NIBSS Person of Interest portal (13,417 listed as of Jan 2026) | Escalate for review | NIBSS and CBN portal (S7) |
| REG-14 | Suspicious transaction report | Analyst confirms suspicion | Screen the alert within 72 hours; file with the NFIU within 24 hours after suspicion is confirmed (S9); the NFIU or EFCC may place a 72-hour stop order | Act 2022 (S4) |

Several of these bind specific institution types: REG-01 to REG-06 target banks, other financial institutions and payment service providers offering instant payments, and REG-08 is a bank-side fee. A tenant's license type should decide which controls apply.

**Added in the second research pass (NFIU, CBN and NIBSS)**

| ID | Control | Trigger | Required action | Basis |
|---|---|---|---|---|
| REG-15 | BVN watchlist | BVN listed in category 0, 1, 2 or 99 (forgery or compromise; confirmed monetary fraud; repeat entries; fugitives) | Block new relationships, ATM, POS, internet and mobile banking, third-party cheques and credit; listing lasts 5 years for category 0 and 10 years for the others | CBN BVN watchlist framework, 2017 (S14); confirm it is still current |
| REG-16 | No-BVN account | Account or wallet without a BVN | Allow inflows only; close the account within 30 days | CBN BVN framework as reported in 2023 (S16) |
| REG-17 | Currency transaction report | Domestic cash transaction of ₦5m or more (individual) or ₦10m or more (corporate) | Generate a CTR in goAML; CTRs are domestic only | NFIU goAML upgrade, effective 1 Feb 2026 (S10) |
| REG-18 | Monthly PEP report | Any transaction by a PEP, whatever the amount | Include in the monthly PEP transaction report | S10 |
| REG-19 | STR classification | STR draft submitted | Require one predicate offence from the 21 defined categories and one matching indicator; choose a category (terrorism financing, kidnapping for ransom, corruption, fraud, drug trafficking or general); goAML rejects incomplete STRs | S10 |

### 3. Fraud and money-laundering typology rules

These 29 rules cover widely recognized fraud and laundering typologies, anchored in what NIBSS reports for Nigeria, and every weight and threshold is a recommended starting value to tune against a tenant's own data.

The patterns follow the NIBSS 2025 digital-payment fraud report (S7): losses of ₦25.85bn across 67,518 cases, with social engineering, SIM swap, account compromise and phishing named as the main techniques, concentrated in internet banking, e-commerce, POS and mobile channels. Weights are points toward the 0-100 score; "Signal" means the rule only adds points, while Step-up and Review mean the rule also sets a minimum decision.

**3.1 Account takeover**

| ID | Rule | Starting trigger | Weight | Action |
|---|---|---|---|---|
| ATO-01 | New device, new beneficiary, then transfer | All three within 30 minutes | 30 | Review |
| ATO-02 | Credential change before transfer | Password, phone or email changed in the last 24 hours, then outbound above the tenant floor | 25 | Step-up |
| ATO-03 | SIM-swap indicator | Telco swap signal or new SIM or device re-registration in the last 72 hours, then a transfer | 25 | Step-up |
| ATO-04 | Device integrity failure | Rooted or jailbroken device, emulator, or hooking framework detected | 20 | Signal |
| ATO-05 | Session anomaly | Concurrent sessions, or IP, user agent or device fingerprint changes mid-session | 20 | Step-up |
| ATO-06 | OTP or PIN failure burst | 3 or more failures in 10 minutes followed by a success | 15 | Signal |

**3.2 Velocity and structuring**

| ID | Rule | Starting trigger | Weight | Action |
|---|---|---|---|---|
| VEL-01 | Outbound burst | 5 or more outbound transfers in 10 minutes | 15 | Signal |
| VEL-02 | Value spike | 24-hour outbound value above 3 times the 30-day daily average and above the tenant floor | 20 | Signal |
| VEL-03 | Beneficiary burst | 3 or more new beneficiaries in 24 hours | 15 | Signal |
| VEL-04 | Structuring | 3 or more transfers in 24 hours, each between 90% and 99.9% of a known limit (₦20,000 cap, ₦5m cash ceiling, ATM limits) | 25 | Review |
| VEL-05 | Repeated round amounts | 3 or more identical round amounts to different beneficiaries in 24 hours | 10 | Signal |
| VEL-06 | Night-time first payment | First high-value transfer to a new beneficiary between 00:00 and 05:00 West Africa Time | 15 | Signal |

**3.3 Mule accounts and layering**

| ID | Rule | Starting trigger | Weight | Action |
|---|---|---|---|---|
| MUL-01 | Pass-through | 80% or more of an inflow leaves within 60 minutes | 25 | Review |
| MUL-02 | Fan-in | 10 or more distinct senders into one account in 24 hours | 20 | Signal |
| MUL-03 | Fan-out | One inflow split to 5 or more beneficiaries within 60 minutes | 20 | Signal |
| MUL-04 | New-account surge | Account under 7 days old with volume above 10 times its KYC-tier norm | 20 | Signal |
| MUL-05 | Dormant reactivation | No activity for 90 days or more, then a large inflow or outflow | 20 | Signal |
| MUL-06 | Shared identifiers | One device across 3 or more accounts, or one BVN, phone or address across multiple accounts or applications | 25 | Review |
| MUL-07 | Reported beneficiary | Beneficiary reported by 3 or more unrelated senders, or on a fraud or person-of-interest list | 30 | Review |
| MUL-08 | Rapid cash-out | Card or peer-to-peer inflow followed by ATM or POS withdrawal within 30 minutes | 15 | Signal |

**3.4 Behavioral anomalies**

| ID | Rule | Starting trigger | Weight | Action |
|---|---|---|---|---|
| BEH-01 | Amount anomaly | Amount more than 3 standard deviations above the 90-day baseline, or above 5 times the customer's maximum | 24 | Signal |
| BEH-02 | Location anomaly | New country or region versus profile (10); impossible travel between consecutive events (20) | 10 / 20 | Signal |
| BEH-03 | Time anomaly | Activity outside the customer's usual hours | 8 | Signal |

**3.5 Card and e-commerce**

| ID | Rule | Starting trigger | Weight | Action |
|---|---|---|---|---|
| CNP-01 | Card testing | 10 or more declines or attempts under ₦500 in 5 minutes on one card, IP or device | 25 | Step-up |
| CNP-02 | Many cards, one device | 5 or more distinct cards on one device or IP in 1 hour | 25 | Review |
| CNP-03 | Merchant anomaly | Merchant refund or chargeback rate far above its own history, or terminal activity from impossible locations | 20 | Signal |

**3.6 Digital lending**

| ID | Rule | Starting trigger | Weight | Action |
|---|---|---|---|---|
| LEN-01 | Loan stacking | 3 or more applications from one BVN or device in 7 days | 20 | Signal |
| LEN-02 | Disburse and cash out | 90% or more of a disbursement leaves within 30 minutes | 20 | Signal |
| LEN-03 | Identity reuse | Same photo, ID document or phone across different applicants | 25 | Review |

### 4. Cybersecurity and platform-abuse rules

Twelve rules watch logins, sessions, APIs and admin actions, so they protect both a tenant's customers and RiskOS itself, and they support the CBN expectation that institutions can detect cyber anomalies.

The CBN 2024 risk-based cybersecurity framework requires institutions to "establish the capacity for monitoring and detecting cyber anomalies or incidents" (S5). These rules are recommended implementations of that duty, not wording from the framework.

| ID | Rule | Starting trigger | Weight | Action |
|---|---|---|---|---|
| SEC-01 | Credential stuffing | 10 or more failed logins from one IP across 5 or more accounts in 5 minutes | 30 | Block IP or challenge |
| SEC-02 | Password spraying | One failed login each on 50 or more accounts from one network in 1 hour | 25 | Challenge |
| SEC-03 | Brute force | 5 or more failed logins on one account in 10 minutes | 20 | Lock or step-up |
| SEC-04 | MFA fatigue | 5 or more push or OTP prompts in 10 minutes | 25 | Pause prompts, alert |
| SEC-05 | Anonymizer use | Tor, known proxy or VPN on an admin or high-value session | 10 | Signal |
| SEC-06 | Bot behavior | Headless browser, inhuman timing or typing speed | 20 | Challenge |
| SEC-07 | Session hijack | IP, user agent or device fingerprint changes mid-session | 25 | Re-authenticate |
| SEC-08 | API key abuse | Key used from a new network or country, request rate 5 times its normal, or a high 4xx and 404 ratio that suggests ID probing | 30 | Alert, throttle |
| SEC-09 | Payout or webhook change | Webhook URL, payout destination or bank account changed, then a high-value transaction within 24 hours | 30 | Review |
| SEC-10 | Privilege escalation | Role change followed by a data export within 1 hour | 30 | Alert, step-up |
| SEC-11 | Account enumeration | 20 or more lookups of nonexistent accounts or IDs from one source in 10 minutes | 20 | Throttle |
| SEC-12 | Mass export | Export volume far above the user's or tenant's baseline | 25 | Step-up, alert |

SEC-07 is the same signal as ATO-05, so score it once, and cap the combined account-takeover signals so one compromised session is not counted several times.

### 5. Governance and reporting presets

The CBN's March 2026 baseline standards for automated AML solutions set eight process requirements that a risk engine must support, and banks must comply by September 2027 and other institutions by March 2028.

The standards cover banks, microfinance banks, payment service providers, money transfer operators, finance companies and mobile money operators, scaled to each institution's size and risk (S1). Implementation roadmaps were due by 10 June 2026.

| ID | Preset | What the product must do | Basis |
|---|---|---|---|
| GOV-01 | Monitoring linked to KYC and KYB | Run multi-scenario monitoring in real or near-real time using customer due-diligence data; a solution without effective CDD, KYC or KYB linkage is not regarded as compliant | CBN baseline standard 5.5 (S1) |
| GOV-02 | Network and peer analysis | Map related parties, run network analysis and compare customers against peer groups | 5.5 |
| GOV-03 | Controlled alert closure | Allow automatic closure only under narrowly defined low-risk conditions; define false-positive and false-negative thresholds; document every tuning decision; notify the CBN where required | 5.5 |
| GOV-04 | Model validation | Independently validate AI and machine-learning models at least annually for accuracy, bias and drift, with explainability | 5.5 |
| GOV-05 | Continuous screening | Screen sanctions, PEP and adverse-media lists at onboarding and continuously, with near-real-time list updates, fuzzy and transliteration matching, and automatic blocks on confirmed matches | 5.3 |
| GOV-06 | Automated reporting | Generate STRs, SARs, CTRs and FTRs in CBN formats, with governance approval before external submission | 5.8 |
| GOV-07 | Tamper-proof audit and access control | Keep audit trails of all system and user activity; enforce role-based access, multi-factor authentication and encryption; comply with the Nigeria Data Protection Act | Baseline standards; S1 |
| GOV-08 | Record retention | Keep customer and transaction records for at least five years after the transaction or end of the relationship | Act 2022 (S4); S1 |

**Recommended internal service levels** (my defaults, not regulation): triage a high-risk alert within 4 hours and reach an STR decision within 12 hours, well inside the NFIU's 72-hour screening allowance and the 24-hour filing clock in REG-14.

**Added in the second research pass**

| ID | Preset | What the product must do | Basis |
|---|---|---|---|
| GOV-09 | No tipping off | Never show STR existence, filing status or internal suspicion flags in API responses, webhooks, customer-facing screens or exports; only compliance roles see them. Employees who warn a subject face a minimum 2 years in prison and a fine of ₦10m or more | NFIU STR guidelines, 18 Dec 2024, citing section 19(2) (S9) |
| GOV-10 | Digital-lender registration check | A tenant of type digital lender must record its FCCPC approval before activation. Existing lenders had 90 days from 21 July 2025 to register; penalties reach ₦100m or 1% of turnover. Banks and other CBN-supervised entities are exempt | FCCPC Digital Lending Regulations 2025 (S13) |
| GOV-11 | Fraud response target | Act on confirmed-fraud alerts within 30 minutes; the CBN states this as its aim for real-time, predictive fraud management | CBN anti-fraud measures, 2026 (S15) |
| GOV-12 | Fraud reporting | Provide a one-click report of confirmed fraud to the industry fraud channel, and track unreported confirmed cases; NIBSS says industry fraud reporting fell about 34% in Q4 2025 and called zero-incident reports unacceptable. I did not find the reporting deadline | NIBSS and CBN (S7, S15) |

### 6. Additional rules by regulator

The second research pass added 16 rules and 4 governance presets from the NFIU, NCC, SEC, FCCPC, CBN and NIBSS; the NCC and SEC items have the thinnest evidence, so treat them as recommendations.

| Regulator | What it contributes | Where it appears |
|---|---|---|
| CBN | Instant-payment guidelines, cash limits, BVN rules, automated AML baseline, 2024 cybersecurity framework, 2026 anti-fraud measures | REG-01 to REG-09, REG-15, REG-16, GOV-01 to GOV-08, GOV-11 |
| NIBSS | Fraud statistics, Person of Interest portal, BVN and NIN identity checks, watchlists, fraud reporting | REG-05, REG-13, GOV-12, section 3 |
| NFIU | STR guidelines, goAML CTR, FTR and PEP reports, STR classification, tipping-off ban | REG-10, REG-14, REG-17 to REG-19, GOV-09 |
| NCC | SIM replacement guidelines (in person only, no phone-based swaps, 12-month register, quarterly audits); NIN-SIM linkage, final deadline 14 Sep 2024 | SIM-01 to SIM-03, ATO-03 |
| SEC | Investments and Securities Act 2025 treats virtual assets as securities; virtual asset providers must monitor transactions and report to the NFIU | CRY-01 to CRY-04 |
| FCCPC | Digital Lending Regulations 2025: lender approval, conduct, penalties | GOV-10, LEN-04 |
| NDPC | Breach notice, impact assessments, annual audits | Section 8 |

| ID | Rule | Starting trigger | Weight | Action | Basis |
|---|---|---|---|---|---|
| INS-01 | Staff-linked account anomaly | Account linked to an employee shows volume far above its peer group, or transacts with customers that employee services | 20 | Review | CBN 2026 staff controls: profiling and lifestyle monitoring (S15) |
| INS-02 | Privileged access then takeover | Staff or admin viewed or changed a customer's contact details or limits, then an outbound transfer from that customer within 24 hours | 30 | Review | S15; recommended |
| SIM-01 | Phone not NIN-verified | Telco or identity provider reports the customer's phone number as not linked to a verified NIN, and the transfer is above the tenant floor | 15 | Signal | NCC NIN-SIM linkage (S11) |
| SIM-02 | Repeated phone changes | Phone number on the profile changed 2 or more times in 30 days | 15 | Signal | NCC SIM replacement guidelines (S11) |
| SIM-03 | Recycled number | Phone number reassigned to a different subscriber while still linked to a profile | 25 | Step-up | S11; recommended |
| CRY-01 | Fiat to crypto to external wallet | Fiat inflow, then crypto purchase, then 80% or more withdrawn to an external wallet within 24 hours | 25 | Review | SEC and NFIU VASP expectations (S12) |
| CRY-02 | Many funding sources | 5 or more distinct cards or accounts fund one account in 7 days, then 80% or more leaves within 24 hours | 25 | Review | S12 |
| CRY-03 | Risky counterparty wallet | Destination wallet flagged by blockchain analytics as sanctioned, mixer, darknet, or an unlicensed provider | 35 | Review or block | S12 |
| CRY-04 | High-value virtual-asset transfer | Transfer above the tenant's configured threshold; the official virtual-asset threshold is still awaited | 15 | Signal | S12 |
| LEN-04 | Third-party repayment | 3 or more repayments in 30 days from accounts whose BVN differs from the borrower's | 15 | Signal | FCCPC context (S13); recommended |
| PON-01 | Ponzi-like pooling | 20 or more distinct unrelated senders into one account in 7 days, and 30% or more of outflows go to accounts that earlier sent funds | 25 | Review | NFIU Ponzi advisory, April 2025 (not opened; S9 note) |

### 7. Rules in plain English

This section restates every rule without technical terms, for compliance officers, MLROs and auditors. It says the same thing as the engineering version in 7B; only the wording differs.

**Two kinds of rule.**

- **Mandatory controls (REG).** These come from law or regulator directives. When one applies, the system acts automatically (decline, hold, block, report) regardless of the risk score.
- **Risk-scoring rules (everything else).** Each one that applies adds points to the customer's risk score, and the score is shown with the reasons. Points do not block anything by themselves. The institution's own decision policy sets what happens at each score level, and a person reviews the high ones.

All amounts and time periods are starting values that each institution can change. Governance presets (GOV) describe how the monitoring programme itself must be run.

**7.1 Mandatory controls**

| Rule | When it applies | What the system does |
|---|---|---|
| REG-01 New-device limit | A customer sends more than ₦20,000 within 24 hours of linking a new phone or app, or within 24 hours of opening the account | Declines the transfer |
| REG-02 One active device | A customer logs in on a second phone while another is already linked to the mobile banking app | Blocks the second login until the customer re-verifies on the new phone |
| REG-03 New-device web login | A customer logs in to internet banking from a device not seen before | Asks for stronger extra verification |
| REG-04 Very large transfers | An individual sends more than ₦25m, or a company more than ₦250m, unless the limit was raised after enhanced checks and extra verification | Blocks the transfer or sends it for enhanced checks |
| REG-05 Suspect BVN hold | The sender's BVN is on a temporary suspicious list | Holds the transfer for up to 24 hours while it is investigated, and opens a case |
| REG-06 Digital account opening | An account is opened or reactivated online without a live-face check, or without live confirmation of the BVN or NIN | Blocks the account opening or reactivation |
| REG-07 Cash payment ceiling | A cash payment above ₦5m (individual) or ₦10m (company) | Flags it; payments above the ceiling must go through a financial institution |
| REG-08 Weekly cash withdrawals | Cash withdrawals across all channels exceed ₦1.5m (individual) or ₦5m (company) in a week | Applies the 3% (individual) or 5% (company) charge to the excess. Banks only; each institution switches it on or off |
| REG-09 ATM limits | ATM withdrawals above ₦100,000 in a day or ₦850,000 in a week for one customer | Declines the withdrawal |
| REG-10 Foreign transfers | An international transfer of US$10,000 or more (or equivalent) | Prepares a report to the NFIU, due within one day of the transaction |
| REG-11 Sanctions match | A name, ID or account matches the Nigerian sanctions list, UN lists or other chosen lists (including spelling variations and transliterations), and the match is confirmed or is strong enough for automatic action | Blocks and freezes, opens a case, and prepares reports to the NFIU and the Nigeria Sanctions Committee |
| REG-12 Politically exposed person | The customer, or an owner behind a company, is a PEP | Sends for review with enhanced checks: source of funds, source of wealth and senior management approval |
| REG-13 Person of interest | The customer or the other party appears on the NIBSS Person of Interest list | Sends for review and opens a case |
| REG-14 Suspicious transaction report (STR) | An alert is still open after 72 hours; or an analyst confirms suspicion | Escalates the old alert to the compliance officer; once suspicion is confirmed, starts a 24-hour clock to file with the NFIU |
| REG-15 BVN watchlist | The customer's BVN is on the CBN watchlist (categories 0, 1, 2 or 99) and the listing is still active (5 years for category 0, 10 years for the others) | Blocks new relationships, ATM, POS, internet and mobile banking, cheques and credit |
| REG-16 Accounts without BVN | An account or wallet has no BVN | Allows money in only; closes the account after 30 days |
| REG-17 Large cash report | A domestic cash deposit, withdrawal or payment of ₦5m or more (individual) or ₦10m or more (company) | Prepares a currency transaction report for the NFIU (domestic cash only) |
| REG-18 PEP monthly report | Any transaction by a PEP, of any size | Adds it to the monthly PEP transaction report |
| REG-19 STR completeness | An STR is submitted without an offence category, a matching indicator, or one of the allowed report types | Stops the submission so the NFIU system does not reject it |

**7.2 Account takeover and unusual spending patterns (risk points)**

| Rule | What we look for | Points |
|---|---|---|
| ATO-01 New phone, new payee, then transfer | Within 30 minutes: a new device is linked, a new payee is added, and money is sent above the institution's minimum | 30 |
| ATO-02 Details changed before a transfer | Password, phone number or email changed in the last 24 hours, then money sent above the minimum | 25 |
| ATO-03 Possible SIM swap | The telco reports a SIM swap, or the SIM or device was re-registered in the last 72 hours, then money is sent | 25 |
| ATO-04 Tampered phone | The phone is rooted, jailbroken, an emulator, or running tools that interfere with the app | 20 |
| ATO-05 Odd session | The same account is open in several places at once, or the location, browser or device changes mid-session | 20 |
| ATO-06 PIN or OTP guessing | Three or more wrong PINs or one-time codes in 10 minutes, then a correct one | 15 |
| VEL-01 Burst of transfers | Five or more transfers out in 10 minutes | 15 |
| VEL-02 Sudden spike | Money sent out in 24 hours is more than three times the customer's usual daily amount, and above the institution's minimum | 20 |
| VEL-03 Many new payees | Three or more new payees added in 24 hours | 15 |
| VEL-04 Splitting to stay under limits | Three or more transfers in 24 hours, each just under a known limit (between 90% and 99.9% of it): the ₦20,000 cap, the cash ceiling or the ATM limits | 25 |
| VEL-05 Repeated round amounts | Three or more identical round amounts to different people in 24 hours | 10 |
| VEL-06 Late-night first payment | A first large payment to a new payee between midnight and 5 a.m. Nigerian time | 15 |

**7.3 Mule accounts, unusual behaviour, cards and lending (risk points)**

| Rule | What we look for | Points |
|---|---|---|
| MUL-01 Money passing straight through | 80% or more of money received leaves within 60 minutes | 25 |
| MUL-02 Many senders into one account | Ten or more different senders pay one account in 24 hours | 20 |
| MUL-03 One payment split many ways | One incoming payment is split among five or more recipients within 60 minutes | 20 |
| MUL-04 New account, big volume | An account under 7 days old moves more than ten times what is normal for its KYC tier | 20 |
| MUL-05 Dormant account wakes up | No activity for 90 days or more, then a large payment in or out | 20 |
| MUL-06 Shared identity details | One device used on three or more accounts, or one BVN, phone number or address on several accounts or applications | 25 |
| MUL-07 Reported recipient | The recipient was reported by three or more unrelated senders, or is on a fraud list or the Person of Interest list | 30 |
| MUL-08 Quick cash-out | Money arrives by card or peer-to-peer and is withdrawn at an ATM or POS within 30 minutes | 15 |
| BEH-01 Unusual amount | The amount is far above the customer's own history (more than 3 standard deviations above their 90-day average, or 5 times their largest), for customers with at least 30 days of history | 24 |
| BEH-02 Unusual location | A country or region the customer does not normally use (10 points), or two events too far apart to travel between in the time (20 points) | 10 or 20 |
| BEH-03 Unusual time | Activity outside the customer's usual hours | 8 |
| CNP-01 Card testing | Ten or more tiny (under ₦500) or declined attempts on one card, network address or device in 5 minutes | 25 |
| CNP-02 Many cards, one device | Five or more different cards used from one device or network address in an hour | 25 |
| CNP-03 Suspicious merchant | A merchant's refunds or chargebacks run at more than three times its own normal rate, or its terminal is active from impossible locations | 20 |
| LEN-01 Loan stacking | Three or more loan applications from one BVN or device in 7 days | 20 |
| LEN-02 Borrow and cash out | 90% or more of a loan is moved out within 30 minutes of payout | 20 |
| LEN-03 Reused identity | The same photo, ID document or phone number appears on different applicants | 25 |
| LEN-04 Repayments from strangers | Three or more repayments in 30 days from accounts whose BVN differs from the borrower's | 15 |

**7.4 Cybersecurity and platform abuse (risk points)**

| Rule | What we look for | Points |
|---|---|---|
| SEC-01 Stolen-password testing | Ten or more failed logins from one network address across five or more accounts in 5 minutes | 30 |
| SEC-02 Password spraying | One failed login each on 50 or more accounts from one network in an hour | 25 |
| SEC-03 Repeated guessing | Five or more failed logins on one account in 10 minutes | 20 |
| SEC-04 Approval-prompt flooding | Five or more push or OTP prompts to one user in 10 minutes | 25 |
| SEC-05 Hidden location | An admin or high-value session uses Tor, a known proxy or a VPN | 10 |
| SEC-06 Automated behaviour | A hidden browser, or typing and timing that is not human | 20 |
| SEC-07 Session takeover | The location changes mid-session together with a change of browser or device | 25 |
| SEC-08 API key misuse | A key is used from a new network or country, at five times its normal rate, or with many failed or not-found requests that suggest guessing IDs | 30 |
| SEC-09 Payout details changed | A webhook address, payout destination or bank account is changed, then a high-value transaction follows within 24 hours | 30 |
| SEC-10 Rights raised, then data exported | A user's role is upgraded and they export data within the hour | 30 |
| SEC-11 Probing for valid accounts | Twenty or more lookups of accounts or IDs that do not exist, from one source in 10 minutes | 20 |
| SEC-12 Unusually large export | An export far above the user's or institution's normal size | 25 |

**7.5 Rules added from the NFIU, NCC, SEC and FCCPC work (risk points)**

| Rule | What we look for | Points |
|---|---|---|
| INS-01 Staff-linked account | An account tied to an employee moves far more than similar accounts, or deals with customers that employee serves. Visible to compliance staff only | 20 |
| INS-02 Staff access, then takeover | Staff viewed or changed a customer's contact details or limits, then money leaves that customer's account within 24 hours | 30 |
| SIM-01 Phone not linked to NIN | The phone number is not linked to a verified NIN, and money is sent above the minimum | 15 |
| SIM-02 Frequent phone changes | The phone number on the profile changed twice or more in 30 days | 15 |
| SIM-03 Recycled number | The phone number now belongs to a different person than the one on the profile | 25 |
| CRY-01 Cash to crypto to outside wallet | Money comes in, is used to buy crypto, and 80% or more is sent to an outside wallet within 24 hours | 25 |
| CRY-02 Many funders, quick exit | Five or more different cards or accounts fund one account in 7 days, then 80% or more leaves within 24 hours | 25 |
| CRY-03 Risky destination wallet | Blockchain analysis flags the receiving wallet as sanctioned, a mixer, darknet or an unlicensed provider | 35 |
| CRY-04 Large crypto transfer | A transfer above the institution's chosen limit (the official SEC limit is still awaited) | 15 |
| PON-01 Ponzi-style pooling | Twenty or more unrelated senders pay one account in 7 days, and 30% or more of what it pays out goes to people who paid in earlier | 25 |

**7.6 How the monitoring programme must be run (governance presets)**

| Rule | What it requires |
|---|---|
| GOV-01 Monitoring tied to customer checks | Monitoring must use KYC and KYB data; a system without that link is not compliant, and the readiness check fails |
| GOV-02 Network and peer analysis | Daily review of how customers are linked, and comparison against similar customers |
| GOV-03 Careful alert closing | Alerts close automatically only when low risk, on an approved list of rules, with tuning decisions documented. Everything else stays open for a person |
| GOV-04 Annual model check | AI and statistical models are independently validated at least every 12 months for accuracy, bias, drift and explainability |
| GOV-05 Continuous screening | Sanctions, PEP and adverse-media checks at onboarding and continuously after, covering customers, owners and counterparties |
| GOV-06 Approved reporting | STRs, SARs, CTRs and FTRs are prepared automatically, but a compliance officer must approve before submission |
| GOV-07 Audit trail and access control | Every action is logged tamper-proof; access is role-based with extra verification, and data is encrypted |
| GOV-08 Record keeping | Records cannot be deleted before five years after the transaction or end of the relationship |
| GOV-09 No tipping off | Nothing about an STR or suspicion flag appears in customer-facing screens, responses, alerts to outside systems or exports; only compliance roles see it |
| GOV-10 Digital lender registration | A digital lender cannot be activated without its FCCPC approval (banks and other CBN-supervised firms are exempt) |
| GOV-11 Fast fraud response | A confirmed-fraud alert not actioned within 30 minutes is escalated |
| GOV-12 Fraud reporting | Confirmed fraud can be reported to the industry channel in one click, and unreported confirmed cases are tracked |

The starting values are for testing on past data before they go live. Set them separately for each customer segment (retail, small business, corporate, agents, new accounts), because one national number flags small customers too often and large ones too rarely. Record the reason for every change; that is also what GOV-03 and the CBN baseline standards ask for.

### 7B. Technical rule logic for engineers (WHEN / IF / THEN)

This section is for engineers; compliance readers can use section 7. Every rule above is written here as executable logic. Rule engines in this space (for example Corsa, SanctionsScanner-style monitoring platforms and the typology libraries banks use) structure a rule the same way: a trigger event, a scope with exclusions, one or more conditions joined by AND/OR, an aggregation over a time window grouped by an entity, and an action. Good practice adds a second, relative condition next to an absolute one (value above the floor AND above a multiple of the customer's own average), segment-specific thresholds, and a written rationale. The syntax below is my own design for RiskOS, not a regulator format. It maps onto the spec's rule model, so Claude Code can turn each block into a JSON rule definition.

**How to read a block.**

- RULE gives the ID, signal weight (or control type), and action.
- WHEN is the event that triggers evaluation.
- SCOPE limits who is checked and lists exclusions.
- IF / AND / OR / ANY OF are the conditions. COUNT, SUM, AVG, DISTINCT take a WINDOW and a BY entity.
- THEN is the outcome: a signal with evidence, a decision override (block, step_up, review, hold) or a task (report, case).
- Money is written in NGN and stored as kobo. All thresholds are tenant-configurable defaults. Hard controls (REG) set a decision directly; typology rules (everything else) only add signal weight, and the decision policy decides.

**7B.1 Regulatory hard controls (REG-01 to REG-19)**

```
RULE REG-01 | control=hard | action=decline
WHEN  txn.type = transfer.outbound
SCOPE tenant.type IN (bank, mfb, psp) AND tenant.instant_payments = true
IF    txn.amount > tenant.cfg.new_device_cap            # default NGN 20_000
AND   ANY OF ( device.bound_age < 24h , account.age < 24h )
THEN  decision=decline  reason=new_device_or_account_cap

RULE REG-02 | control=hard | action=block_session
WHEN  session.login
SCOPE channel = mobile_app
IF    device.id != account.active_device_id
AND   account.active_device_id IS NOT NULL
THEN  decision=block_session  require=fresh_auth_on_new_device  reason=single_active_device

RULE REG-03 | control=hard | action=step_up
WHEN  session.login
SCOPE channel = web
IF    device.is_new = true
THEN  decision=step_up  require=enhanced_mfa  reason=new_device_web_login

RULE REG-04 | control=hard | action=block_or_edd
WHEN  txn.type = transfer.outbound
IF    ( customer.type = individual AND txn.amount > NGN 25_000_000 )
   OR ( customer.type = corporate  AND txn.amount > NGN 250_000_000 )
UNLESS account.limit_raised = true AND account.limit_raise.edd_done = true AND account.limit_raise.mfa_done = true
THEN  decision=block  route=edd_queue  reason=high_value_ceiling

RULE REG-05 | control=hard | action=hold
WHEN  txn.type = transfer.outbound
IF    customer.bvn IN watchlist.temporary_suspect_bvn
THEN  decision=hold  hold_max=24h  case=open  reason=suspect_bvn_hold

RULE REG-06 | control=hard | action=block
WHEN  account.open OR account.reactivate
SCOPE channel = digital
IF    kyc.liveness_passed = false
   OR kyc.bvn_live_check != verified
   OR kyc.nin_live_check != verified
THEN  decision=block  reason=digital_onboarding_identity_incomplete

RULE REG-07 | control=hard | action=flag
WHEN  txn.type = cash.payment
IF    ( customer.type = individual AND txn.amount > NGN 5_000_000 )
   OR ( customer.type = corporate  AND txn.amount > NGN 10_000_000 )
THEN  flag=cash_ceiling  require=route_via_financial_institution

RULE REG-08 | control=hard | action=fee | toggle=tenant.type=bank
WHEN  txn.type = cash.withdrawal
SCOPE tenant.type = bank AND tenant.cfg.cash_fee_enabled = true
WINDOW 7d (calendar week)  BY customer.id
IF    SUM(txn.amount WHERE channel=any) > ( individual ? NGN 1_500_000 : NGN 5_000_000 )
THEN  fee = ( individual ? 3% : 5% ) * excess  reason=weekly_cash_limit

RULE REG-09 | control=hard | action=decline
WHEN  txn.type = atm.withdrawal
IF    SUM(txn.amount) OVER 1d  BY customer.id > NGN 100_000
   OR SUM(txn.amount) OVER 7d  BY customer.id > NGN 850_000
THEN  decision=decline  reason=atm_limit

RULE REG-10 | control=hard | action=report
WHEN  txn.type = transfer.international
IF    txn.amount_usd_equiv >= 10_000
THEN  task=report.FTR  due=1d_after_txn  to=NFIU

RULE REG-11 | control=hard | action=block
WHEN  txn.created OR customer.onboarded OR list.updated
IF    screen(party.name, party.id, party.account) MATCHES sanctions_lists
        USING fuzzy + transliteration  score >= tenant.cfg.match_threshold
AND   match.reviewed = confirmed OR tenant.cfg.auto_block_on_strong_match = true
THEN  decision=block  freeze=account  case=open  task=report.NFIU,NigSaC  reason=sanctions_match

RULE REG-12 | control=hard | action=edd
WHEN  customer.onboarded OR customer.periodic_review OR txn.created
IF    customer.is_pep = true OR ANY(beneficial_owner.is_pep = true)
THEN  decision=review  require=edd,source_of_funds,source_of_wealth,senior_approval  reason=pep

RULE REG-13 | control=hard | action=review
WHEN  txn.created OR customer.onboarded
IF    customer.id OR counterparty.id IN nibss.person_of_interest
THEN  decision=review  case=open  reason=person_of_interest

RULE REG-14 | control=hard | action=report
WHEN  alert.created
IF    alert.age > 72h AND alert.status = open
THEN  escalate=compliance_officer  reason=screening_sla_breach
WHEN  alert.suspicion_confirmed
THEN  task=report.STR  due=24h  to=NFIU  gov=GOV-09

RULE REG-15 | control=hard | action=block
WHEN  customer.onboarded OR txn.created OR credit.requested OR session.login
IF    customer.bvn.watchlist_category IN (0, 1, 2, 99)
AND   watchlist.entry.active = true       # category 0 lasts 5y, others 10y
THEN  decision=block  scope=(new_relationship, atm, pos, web, mobile, cheque, credit)  reason=bvn_watchlist

RULE REG-16 | control=hard | action=restrict
WHEN  account.created OR daily_job
IF    account.bvn IS NULL
THEN  restrict=inflow_only
AND   IF account.age >= 30d THEN task=close_account

RULE REG-17 | control=hard | action=report
WHEN  txn.type IN (cash.deposit, cash.withdrawal, cash.payment)
SCOPE txn.domestic = true
IF    ( customer.type = individual AND txn.amount >= NGN 5_000_000 )
   OR ( customer.type = corporate  AND txn.amount >= NGN 10_000_000 )
THEN  task=report.CTR  format=goAML

RULE REG-18 | control=hard | action=report
WHEN  txn.created
IF    customer.is_pep = true
THEN  add_to=monthly_pep_report  amount_floor=none

RULE REG-19 | control=hard | action=validate
WHEN  report.STR.submit
IF    report.predicate_offence IS NULL
   OR report.indicator IS NULL
   OR report.category NOT IN (terrorism_financing, kidnapping_ransom, corruption, fraud, drug_trafficking, general)
THEN  reject_submit  reason=goaml_would_reject
```

**7B.2 Account takeover and velocity (ATO-01 to ATO-06, VEL-01 to VEL-06)**

```
RULE ATO-01 | weight=30 | action=signal
WHEN  txn.type = transfer.outbound
WINDOW 30m BY account.id
IF    EXISTS(device.bound  WHERE device.is_new = true)
AND   EXISTS(beneficiary.added WHERE beneficiary.is_new = true)
AND   txn.amount > tenant.cfg.floor
THEN  signal=ATO-01  evidence=(device.id, beneficiary.id, times)

RULE ATO-02 | weight=25
WHEN  txn.type = transfer.outbound
IF    ANY OF ( EXISTS(profile.password_changed OVER 24h) ,
               EXISTS(profile.phone_changed    OVER 24h) ,
               EXISTS(profile.email_changed    OVER 24h) )
AND   txn.amount > tenant.cfg.floor
THEN  signal=ATO-02  evidence=(change_type, change_time)

RULE ATO-03 | weight=25
WHEN  txn.type = transfer.outbound
IF    ANY OF ( telco.sim_swap_flag = true , EXISTS(sim.reregistered OVER 72h) , EXISTS(device.reregistered OVER 72h) )
THEN  signal=ATO-03  evidence=(swap_source, swap_time)

RULE ATO-04 | weight=20
WHEN  session.login OR txn.created
IF    ANY OF ( device.rooted = true , device.jailbroken = true , device.emulator = true , device.hooking_framework = true )
THEN  signal=ATO-04  evidence=(integrity_flags)

RULE ATO-05 | weight=20
WHEN  session.activity
IF    ANY OF ( COUNT(DISTINCT session.id WHERE open = true) > 1 ,
               session.ip != session.start_ip ,
               session.user_agent != session.start_user_agent ,
               session.device_fp != session.start_device_fp )
THEN  signal=ATO-05  evidence=(changed_attributes)

RULE ATO-06 | weight=15
WHEN  auth.success
WINDOW 10m BY account.id
IF    COUNT(auth.failure WHERE type IN (otp, pin)) >= 3
THEN  signal=ATO-06

RULE VEL-01 | weight=15
WHEN  txn.type = transfer.outbound
WINDOW 10m BY account.id
IF    COUNT(txn) >= 5
THEN  signal=VEL-01  evidence=(count, window)

RULE VEL-02 | weight=20
WHEN  txn.type = transfer.outbound
WINDOW 24h BY account.id
IF    SUM(txn.amount) > 3 * AVG(daily_outbound OVER 30d)     # relative condition
AND   SUM(txn.amount) > tenant.cfg.floor                     # absolute condition
THEN  signal=VEL-02  evidence=(sum_24h, avg_30d)

RULE VEL-03 | weight=15
WHEN  beneficiary.added
WINDOW 24h BY account.id
IF    COUNT(beneficiary.added WHERE is_new = true) >= 3
THEN  signal=VEL-03

RULE VEL-04 | weight=25
WHEN  txn.created
WINDOW 24h BY account.id
IF    COUNT(txn WHERE txn.amount BETWEEN 0.90*L AND 0.999*L) >= 3
      WHERE L IN ( tenant.cfg.new_device_cap , cash_ceiling , atm_daily_limit , atm_weekly_limit )
THEN  signal=VEL-04  evidence=(limit_hit, amounts)   # structuring

RULE VEL-05 | weight=10
WHEN  txn.type = transfer.outbound
WINDOW 24h BY account.id
IF    COUNT(txn WHERE txn.amount % 1_000 = 0 AND txn.amount = same AND DISTINCT beneficiary.id) >= 3
THEN  signal=VEL-05

RULE VEL-06 | weight=15
WHEN  txn.type = transfer.outbound
IF    beneficiary.is_new = true
AND   txn.amount > tenant.cfg.high_value
AND   local_time(txn.ts, Africa/Lagos) BETWEEN 00:00 AND 05:00
AND   COUNT(txn WHERE beneficiary = same OVER ALL) = 0     # first payment
THEN  signal=VEL-06
```

**7B.3 Mule, behavior, card and lending (MUL-01 to MUL-08, BEH-01 to BEH-03, CNP-01 to CNP-03, LEN-01 to LEN-04)**

```
RULE MUL-01 | weight=25
WHEN  txn.type = transfer.outbound
WINDOW 60m BY account.id
IF    SUM(txn.amount WHERE direction=out) >= 0.80 * SUM(txn.amount WHERE direction=in)
AND   SUM(txn.amount WHERE direction=in) > tenant.cfg.floor
THEN  signal=MUL-01  evidence=(in_sum, out_sum, ratio)

RULE MUL-02 | weight=20
WHEN  txn.type = transfer.inbound
WINDOW 24h BY account.id
IF    COUNT(DISTINCT sender.id) >= 10
THEN  signal=MUL-02

RULE MUL-03 | weight=20
WHEN  txn.type = transfer.outbound
WINDOW 60m BY source_inflow.id
IF    COUNT(DISTINCT beneficiary.id) >= 5
THEN  signal=MUL-03

RULE MUL-04 | weight=20
WHEN  txn.created
SCOPE account.age < 7d
IF    SUM(txn.amount OVER account.age) > 10 * kyc_tier.norm(account.kyc_tier)
THEN  signal=MUL-04  evidence=(volume, tier_norm)

RULE MUL-05 | weight=20
WHEN  txn.created
IF    account.days_since_last_activity >= 90
AND   txn.amount > tenant.cfg.large_after_dormancy
THEN  signal=MUL-05

RULE MUL-06 | weight=25
WHEN  account.created OR application.submitted OR txn.created
IF    ANY OF ( COUNT(DISTINCT account.id BY device.id)  >= 3 ,
               COUNT(DISTINCT account.id BY bvn)        >= 2 ,
               COUNT(DISTINCT account.id BY phone)      >= 2 ,
               COUNT(DISTINCT account.id BY address)    >= 2 )
THEN  signal=MUL-06  evidence=(shared_identifier, linked_accounts)

RULE MUL-07 | weight=30
WHEN  txn.type = transfer.outbound
IF    ANY OF ( COUNT(DISTINCT reporter.id WHERE report.beneficiary = txn.beneficiary) >= 3 ,
               beneficiary IN fraud_list , beneficiary IN nibss.person_of_interest )
THEN  signal=MUL-07

RULE MUL-08 | weight=15
WHEN  txn.type IN (atm.withdrawal, pos.purchase)
WINDOW 30m BY account.id
IF    EXISTS(inflow WHERE source IN (card, p2p))
AND   txn.amount >= 0.70 * inflow.amount
THEN  signal=MUL-08

RULE BEH-01 | weight=24
WHEN  txn.created
IF    ANY OF ( txn.amount > AVG(txn.amount OVER 90d) + 3 * STDDEV(txn.amount OVER 90d) ,
               txn.amount > 5 * MAX(txn.amount OVER 90d) )
AND   customer.history_days >= 30                            # cold-start exclusion
THEN  signal=BEH-01

RULE BEH-02 | weight=10 or 20
WHEN  session.login OR txn.created
IF    geo.country NOT IN profile.usual_countries THEN signal=BEH-02 weight=10
IF    distance(geo, previous.geo) / time_between > 900 km/h THEN signal=BEH-02 weight=20   # impossible travel

RULE BEH-03 | weight=8
WHEN  txn.created
IF    local_hour(txn.ts) NOT IN profile.usual_hours  AND  profile.history_days >= 30
THEN  signal=BEH-03

RULE CNP-01 | weight=25
WHEN  card.authorization
WINDOW 5m BY card.id OR ip OR device.id
IF    COUNT(auth WHERE amount < NGN 500 OR result = declined) >= 10
THEN  signal=CNP-01

RULE CNP-02 | weight=25
WHEN  card.authorization
WINDOW 1h BY device.id OR ip
IF    COUNT(DISTINCT card.id) >= 5
THEN  signal=CNP-02

RULE CNP-03 | weight=20
WHEN  merchant.daily_job OR terminal.activity
IF    merchant.refund_rate  > 3 * merchant.refund_rate_baseline
   OR merchant.chargeback_rate > 3 * merchant.chargeback_rate_baseline
   OR terminal.impossible_location = true
THEN  signal=CNP-03  scope=merchant

RULE LEN-01 | weight=20
WHEN  loan.application.submitted
WINDOW 7d BY bvn OR device.id
IF    COUNT(application) >= 3
THEN  signal=LEN-01

RULE LEN-02 | weight=20
WHEN  txn.type = transfer.outbound
WINDOW 30m BY account.id
IF    EXISTS(loan.disbursement)
AND   SUM(txn.amount) >= 0.90 * loan.disbursement.amount
THEN  signal=LEN-02

RULE LEN-03 | weight=25
WHEN  loan.application.submitted
IF    ANY OF ( photo_hash IN other_applicants , id_document_hash IN other_applicants , phone IN other_applicants )
THEN  signal=LEN-03

RULE LEN-04 | weight=15
WHEN  loan.repayment
WINDOW 30d BY loan.id
IF    COUNT(repayment WHERE payer.bvn != borrower.bvn) >= 3
THEN  signal=LEN-04
```

**7B.4 Cybersecurity and platform abuse (SEC-01 to SEC-12)**

```
RULE SEC-01 | weight=30
WHEN  auth.failure
WINDOW 5m BY ip
IF    COUNT(auth.failure) >= 10 AND COUNT(DISTINCT account.id) >= 5
THEN  signal=SEC-01  action_hint=rate_limit_ip

RULE SEC-02 | weight=25
WHEN  auth.failure
WINDOW 1h BY network(asn OR /24)
IF    COUNT(DISTINCT account.id WHERE failures_per_account = 1) >= 50
THEN  signal=SEC-02

RULE SEC-03 | weight=20
WHEN  auth.failure
WINDOW 10m BY account.id
IF    COUNT(auth.failure) >= 5
THEN  signal=SEC-03  action_hint=temporary_lock

RULE SEC-04 | weight=25
WHEN  mfa.prompt_sent
WINDOW 10m BY account.id
IF    COUNT(mfa.prompt_sent WHERE type IN (push, otp)) >= 5
THEN  signal=SEC-04  action_hint=pause_prompts

RULE SEC-05 | weight=10
WHEN  session.login OR txn.created
SCOPE ( user.role = admin OR txn.amount > tenant.cfg.high_value )
IF    ip.is_tor = true OR ip.is_known_proxy = true OR ip.is_vpn = true
THEN  signal=SEC-05

RULE SEC-06 | weight=20
WHEN  session.activity
IF    ANY OF ( client.headless = true , input.keystroke_interval_ms < tenant.cfg.min_human , input.timing_variance < tenant.cfg.min_variance )
THEN  signal=SEC-06

RULE SEC-07 | weight=25
WHEN  session.activity
IF    session.ip != session.start_ip
AND   ANY OF ( session.user_agent != start_user_agent , session.device_fp != start_device_fp )
THEN  signal=SEC-07  action_hint=revoke_session

RULE SEC-08 | weight=30
WHEN  api.request
SCOPE auth.type = api_key
IF    ANY OF ( request.network NOT IN key.known_networks ,
               request.country NOT IN key.known_countries ,
               RATE(request OVER 5m) > 5 * key.baseline_rate ,
               RATIO(status IN (400, 404) OVER 10m) > tenant.cfg.probe_ratio )
THEN  signal=SEC-08  action_hint=throttle_key

RULE SEC-09 | weight=30
WHEN  txn.created
WINDOW 24h BY tenant.id OR merchant.id
IF    EXISTS(config.changed WHERE field IN (webhook_url, payout_destination, bank_account))
AND   txn.amount > tenant.cfg.high_value
THEN  signal=SEC-09  action_hint=hold_payout

RULE SEC-10 | weight=30
WHEN  data.export
WINDOW 1h BY user.id
IF    EXISTS(user.role_changed)
THEN  signal=SEC-10

RULE SEC-11 | weight=20
WHEN  api.lookup
WINDOW 10m BY source(ip OR api_key)
IF    COUNT(lookup WHERE result = not_found) >= 20
THEN  signal=SEC-11

RULE SEC-12 | weight=25
WHEN  data.export
IF    export.rows > 5 * BASELINE(export.rows BY user.id OVER 30d)
   OR export.rows > 5 * BASELINE(export.rows BY tenant.id OVER 30d)
THEN  signal=SEC-12
```

**7B.5 Rules added by regulator (INS, SIM, CRY, PON)**

```
RULE INS-01 | weight=20
WHEN  txn.created
SCOPE account.linked_to_employee = true
IF    SUM(txn.amount OVER 30d) > peer_group.p95(SUM OVER 30d)
   OR EXISTS(txn WHERE counterparty.customer_id IN employee.serviced_customers)
THEN  signal=INS-01  visibility=compliance_only   # gov=GOV-09

RULE INS-02 | weight=30
WHEN  txn.type = transfer.outbound
WINDOW 24h BY customer.id
IF    EXISTS(staff_action WHERE type IN (view_contact, change_contact, change_limits))
THEN  signal=INS-02  evidence=(staff.id, action, time)

RULE SIM-01 | weight=15
WHEN  txn.type = transfer.outbound
IF    identity.phone_nin_linked = false
AND   txn.amount > tenant.cfg.floor
THEN  signal=SIM-01

RULE SIM-02 | weight=15
WHEN  profile.phone_changed
WINDOW 30d BY customer.id
IF    COUNT(profile.phone_changed) >= 2
THEN  signal=SIM-02

RULE SIM-03 | weight=25
WHEN  telco.lookup OR txn.created
IF    telco.msisdn_subscriber_id != profile.phone_subscriber_id
THEN  signal=SIM-03

RULE CRY-01 | weight=25
WHEN  crypto.withdrawal
WINDOW 24h BY account.id
IF    EXISTS(fiat.inflow) AND EXISTS(crypto.purchase)
AND   SUM(crypto.withdrawal.external) >= 0.80 * SUM(crypto.purchase)
THEN  signal=CRY-01

RULE CRY-02 | weight=25
WHEN  txn.type = transfer.outbound
IF    COUNT(DISTINCT funding_source.id OVER 7d BY account.id) >= 5
AND   SUM(outflow OVER 24h) >= 0.80 * SUM(inflow OVER 24h)
THEN  signal=CRY-02

RULE CRY-03 | weight=35
WHEN  crypto.withdrawal
IF    analytics.wallet_tag IN (sanctioned, mixer, darknet, unlicensed_provider)
THEN  signal=CRY-03  action_hint=hold_and_review

RULE CRY-04 | weight=15
WHEN  crypto.transfer
IF    txn.amount > tenant.cfg.va_threshold      # official SEC threshold awaited
THEN  signal=CRY-04

RULE PON-01 | weight=25
WHEN  txn.type = transfer.inbound
WINDOW 7d BY account.id
IF    COUNT(DISTINCT sender.id WHERE unrelated = true) >= 20
AND   SUM(outflow WHERE recipient IN prior_senders) >= 0.30 * SUM(outflow)
THEN  signal=PON-01
```

**7B.6 Governance presets (GOV-01 to GOV-12)**

```
RULE GOV-01 | type=policy
IF    tenant.monitoring.enabled = true AND tenant.kyc_data.linked = false
THEN  readiness=fail  reason=monitoring_without_cdd_linkage

RULE GOV-02 | type=job | schedule=daily
THEN  build(related_party_graph)  AND  compare(customer, peer_group)  emit=network_signals

RULE GOV-03 | type=policy
WHEN  alert.auto_close_requested
IF    alert.risk_level = LOW AND rule.in(tenant.cfg.auto_close_allowlist) AND tuning.documented = true
THEN  allow_auto_close  ELSE keep_open

RULE GOV-04 | type=job | schedule=annual
IF    model.last_validated > 12mo
THEN  readiness=fail  require=independent_validation(accuracy, bias, drift, explainability)

RULE GOV-05 | type=control
WHEN  customer.onboarded OR list.updated OR daily_job
THEN  run REG-11  AND  REG-12  AND  adverse_media_screen  FOR customer, beneficial_owner, counterparty

RULE GOV-06 | type=workflow
WHEN  report.draft_ready  (STR, SAR, CTR, FTR)
IF    report.approved_by IN roles(compliance_officer)
THEN  allow_submit  ELSE hold_for_approval

RULE GOV-07 | type=control
WHEN  any.state_change
THEN  append(audit_log, hash_chain)  AND  enforce(rbac, mfa)  AND  encrypt(at_rest, in_transit)

RULE GOV-08 | type=control
WHEN  record.delete_requested
IF    record.age < 5y_after(txn OR relationship_end)
THEN  deny_delete  reason=retention

RULE GOV-09 | type=control
WHEN  api.response OR webhook.send OR export.generate
IF    payload CONTAINS (str.exists OR str.status OR flag.visibility = compliance_only)
AND   recipient.role NOT IN (compliance_officer, mlro)
THEN  strip_fields  AND  log=audit

RULE GOV-10 | type=policy
WHEN  tenant.activate
IF    tenant.type = digital_lender AND tenant.fccpc_approval IS NULL AND tenant.regulator != CBN
THEN  deny_activate  reason=missing_fccpc_registration

RULE GOV-11 | type=sla
WHEN  alert.fraud_confirmed
IF    NOW - alert.confirmed_at > 30m AND alert.actioned = false
THEN  escalate  reason=fraud_response_target

RULE GOV-12 | type=workflow
WHEN  alert.fraud_confirmed
THEN  offer(report_to_industry_channel)  AND  track(unreported_confirmed_cases)
```

These are defaults for backtesting in a sandbox before they go live. Calibrate thresholds per segment (retail, SME, corporate, agent banking, new accounts) because one global number overflags small customers and underflags large ones. Record the rationale for every changed threshold, which is also what GOV-03 and the CBN's baseline standards ask for.

### 8. Legal clocks that become product requirements

Nigerian rules set deadlines of 24 hours or 72 hours for most reporting duties, so RiskOS needs timers, templates and an audit trail for each of them.

| Clock | Duty | Who it applies to | Product requirement |
|---|---|---|---|
| 24 hours | Report cyber incidents to the CBN | Banks and payment service banks under the CBN 2024 risk-based cybersecurity framework (S5) | Incident record with a countdown from detection and an export for the CBN |
| 72 hours | Report a cyber threat or incident to the national or sectoral CERT (ngCERT, NigFinCERT) | Organizations under the Cybercrimes Act as amended in 2024 (S5) | Same incident record, second clock |
| 72 hours | Notify the data protection regulator of a personal-data breach likely to risk people's rights; notify affected people immediately if risk is high | Data controllers under the Nigeria Data Protection Act 2023 (S6) | Breach workflow listing what was exposed, who is affected and what was done |
| 24 hours | Screen an alert within 72 hours; file the suspicious transaction report within 24 hours after suspicion is confirmed (S9). Older summaries of the Act say 24 hours from the transaction | Financial institutions (S4) | Two case timers: 72 hours from alert creation, 24 hours from confirmed suspicion; keep both configurable (see section 9) |
| 1 day | Report international transfers of US$10,000 or more | Financial institutions (S4) | Automatic report generation (REG-10) |
| 72 hours | Maximum stop order by the NFIU or EFCC | Any account or transaction in scope (S4) | Account hold with an automatic expiry and audit entry |
| 24 hours | Investigation window for a suspect BVN before a transfer completes | Instant-payment providers (S2) | Held-transfer queue (REG-05) |

**Other obligations to design for**

- **Data protection impact assessment:** required for large-scale or high-risk processing, including systematic evaluation and sensitive data; the guidance sets four months for new sensitive-data software and six months for existing processing (S6).
- **Major-importance registration and annual audit:** organizations classed as ultra-high or extra-high level register as data controllers or processors of major importance and file an annual compliance audit return by 31 March through a licensed organization; late filing carries a 50% administrative penalty (S6).
- **Automated decisions:** people keep rights around automated processing, including transparency and a route to human intervention (S6). The RiskOS decision path should return explainable signals and support a human review path.
- **Cybersecurity framework dates:** compliance date 1 July 2024, annual self-assessment due 28 February, quarterly board reports that include vulnerability and penetration-test results, and third-party contracts that give audit rights and require standards such as PCI DSS and ISO 27001 (S5).
- **Sensitive data:** the 72-hour breach duty explicitly covers items such as login credentials, BVN and NIN (S5).

### 9. Gaps, conflicts and sources

Verify the CBN figures against the circulars and settle five open points with counsel before any rule in section 2 ships.

**What to verify or decide**

1. **CBN circulars not read directly.** The CBN site returned access errors, so REG-01 to REG-06 (instant payment guidelines of 12 March 2026), REG-08 and REG-09 (cash policy), and GOV-01 to GOV-08 (baseline standards of 10 March 2026) rest on law-firm and press summaries. Confirm amounts, scope and effective dates in the originals.
2. **KYC tier limits conflict, so none are hard-coded.** The 2013 and 2016 circulars give tier 1 as a ₦50,000 single deposit and ₦300,000 balance; a 2017 mobile-money circular gives daily limits of ₦50,000, ₦200,000 and ₦5,000,000; one 2026 guide gives ₦30,000 and ₦500,000 for tiers 1 and 2. Keep tier limits as per-institution configuration, and use the current CBN circular for each customer.
3. **Cash ceiling versus reporting threshold.** The ₦5m and ₦10m figures in the 2022 Act are ceilings on cash payments. The NFIU's goAML upgrade (S10) puts the currency transaction report threshold at the same ₦5m and ₦10m for domestic cash (REG-17), but the Act itself does not say so, so keep both values configurable.
4. **Sanctions freeze timing.** Reports say freezes are "immediate" under the 2022 terrorism act but give no hour count; confirm the legal timeline.
5. **Applicability.** Several controls bind banks, payment service providers or specific license types. Capture each tenant's license type so the right pack turns on.
6. **Suspicious-report timing differs by source.** The NFIU guidelines (S9) say 72 hours to screen and 24 hours to file after suspicion is confirmed; the 2022 Act summaries (S4) say 24 hours from the transaction; SEC and Templars material for virtual-asset providers (S12) mentions 48 hours. REG-14 follows the NFIU; confirm with counsel.
7. **Person of Interest count.** One report says 3,417 entries and another 13,417 (REG-13 uses the latter). The count does not change the rule, but check it against the portal.
8. **Retention.** The Act says five years; some summaries say seven for certain records. GOV-08 uses five as the minimum and should be configurable upward.
9. **Items I could not confirm.** The official SEC virtual-asset threshold (CRY-04 stays a tenant setting), any NCC rule on activity after a SIM swap (SIM-01 to SIM-03 are my design from the SIM replacement and NIN-linking rules, S11), the NFIU's Ponzi-scheme advisory (PON-01 is a typology rule, not a quoted threshold), the full FCCPC digital-lending text (S13 is thin), and whether the 2017 BVN watchlist circular (S14) is still the current version.
10. **Rule logic is my design.** Section 7B expresses each rule in notation I chose, using thresholds from sections 2 to 6. It is not a regulator format, and it has not been backtested on Nigerian data.

**Not covered here:** the cybersecurity levy, telecom and SIM-registration rules, securities and virtual-asset regulation, PCI DSS specifics, NFIU report formats, and data-retention periods under the data protection law.

**Sources**

- S1: CBN baseline standards for automated AML solutions. [CBN circular (access blocked)](https://www.cbn.gov.ng/Out/2026/CCD/CBN%20issues%20Baseline%20Standards%20for%20Automated%20Anti-Money%20Laundering%20Solution.pdf), [Acelera summary](https://www.acelera.law/publications/cbn-issues-baseline-standards-for-automated-aml-solutions-a-new-compliance-paradigm-for-financial-institutions), [CBN Baseline Standards guide](https://cbnbaselinestandards.com/)
- S2: CBN instant payment guidelines. [Afriwise](https://www.afriwise.com/blog/device-binding-and-transaction-caps-the-cbns-instant-payment-guidelines-take-effect)
- S3: CBN cash policy from 1 January 2026. [AllAfrica](https://allafrica.com/stories/202512040118.html)
- S4: Money Laundering (Prevention and Prohibition) Act 2022. [Mondaq key provisions](https://mondaq.com/nigeria/money-laundering/1233474/key-provisions-in-the-money-laundering-prevention-and-prohibition-act-2022), [Mondaq compliance requirements](https://www.mondaq.com/nigeria/money-laundering/1468536/compliance-requirements-in-nigerias-anti-money-laundering-landscape)
- S5: Cybersecurity. [Mondaq on the CBN 2024 framework](https://www.mondaq.com/article/1518574), [ALN on incident reporting](https://aln.africa/insight/incident-response-beyond-the-organisation-a-brief-overview-on-cybersecurity-incident-reporting-obligations-under-nigerian-law/)
- S6: Data protection. [LawPavilion on the NDPA and GAID 2025](https://blog.lawpavilion.com/ndp-act-2023-gaid-2025-a-comprehensive-guide-to-nigerias-new-data-protection-landscape)
- S7: Fraud. [Techeconomy on NIBSS 2025 fraud](https://techeconomy.ng/nibss-digital-payment-fraud-drops-51-to-%e2%82%a625-85b-in-2025), [AllAfrica on the Person of Interest portal](https://allafrica.com/stories/202601270398.html), [TechCabal on CBN and NIBSS fraud measures](https://techcabal.com/2025/01/28/cbn-nibss-fraud/)
- S8: Sanctions. [BusinessDay on the Nigeria sanctions list](https://businessday.ng/news/article/fg-names-48-suspected-terror-financiers-orders-asset-freeze/)
- KYC tier conflict: [BusinessDay on tiered KYC](https://businessday.ng/business-economy/article/cbn-seeks-to-expand-banking-net-in-new-tiered-kyc-requirements/), [MFW4A on mobile-money limits](https://www.mfw4a.org/news/nigeria-cbn-reviews-limits-mobile-money-transaction-balance), [Youverify KYC guide](https://youverify.co/en/blogs/cbn-kyc-aml-requirements-2026)
- S9: NFIU STR guidelines. [Proshare](https://www.proshare.co/articles/nfiu-issues-new-guidelines-for-financial-institutions-to-strengthen-suspicious-transaction-reporting?menu=Regulators&classification=Read&category=Regulators)
- S10: NFIU goAML upgrade (CTR, FTR, PEP reports). [Mondaq](https://www.mondaq.com/nigeria/financial-services/1727820/nfiu-upgrades-goaml-platform-key-updates-reporting-entities-must-note)
- S11: NCC. [Techpoint on SIM replacement guidelines](https://techpoint.africa/?p=164852), [TheCable on NIN-SIM linking](https://www.thecable.ng/?p=1074708)
- S12: SEC and virtual assets. [Mondaq](https://www.mondaq.com/nigeria/financial-services/1827544/aml-compliance-in-nigerias-virtual-asset-sectorwhat-market-players-should-know), [Templars](https://www.templars-law.com/app/uploads/2026/08/Anti-Money-Laundering-Compliance-in-Nigerias-Virtual-Asset-Sector.pdf)
- S13: FCCPC digital lending. [Mondaq](https://webiis10.mondaq.com/consumer-law/1792304/fccpc-digital-lending-regulations-2025-what-it-means-for-digital-lenders), [Guardian](https://guardian.ng/news/digital-lending-fccpc-issues-regulations-to-tackle-consumer-abuses/)
- S14: CBN BVN watchlist framework (2017). [CBN circular](https://www.cbn.gov.ng/Out/2017/BPSD/Circular%20on%20the%20Regulatory%20Framework%20for%20BVN%20%20Watchlist%20for%20Nigerian%20Financial%20System.pdf)
- S15: CBN anti-fraud measures 2026. [AllAfrica](https://allafrica.com/stories/202601230595.html)
- S16: No-BVN accounts closed after 30 days. [Techeconomy](https://v2.techeconomy.ng/new-accounts-without-bvn-to-be-blocked-in-30-days-says-cbn)
- Rule-logic conventions: [Corsa](https://docs.corsa.finance/transaction-monitoring/building-rules), [SanctionsScanner](https://www.sanctionscanner.com/blog/transaction-monitoring-rules-1371)
