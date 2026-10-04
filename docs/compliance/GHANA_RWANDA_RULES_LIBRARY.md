# Codifying Ghana and Rwanda compliance rules for RiskOS

Ghana and Rwanda each get a full preset library for RiskOS: 68 Ghana rules and 64 Rwanda rules (132 in total), every one written in plain English for compliance officers and as WHEN/IF/THEN logic for engineers, reusing the Nigeria library's rule IDs, table format and notation. The headline legal clocks are tighter than Nigeria's in places: Ghana requires an STR within **24 hours** of forming suspicion, a CTR at **GHS 50,000** for banks (GHS 20,000 for specialised deposit-taking institutions and forex bureaux) and an electronic transfer report at **US$1,000**, not US$10,000 ([FIC](https://fic.gov.gh/report-types.html); [Act 1044](https://ghalii.org/akn/gh/act/2020/1044/eng@2020-12-29)). Rwanda defines "immediately" as **24 hours** for sanctions freezes, keeps records **10 years** after a relationship ends, requires personal-data breach notice to the National Cyber Security Authority within **48 hours**, and sets cross-border cash declaration above **US$10,000** ([Law 001/2025](https://regalert.today/document/195f62e1-4ddf-4323-86ae-c8a19628da31/text); [ESAAMLG](https://www.esaamlg.org/reports/Rwanda%20FUR-August%202025.pdf); [Reg 002/FIC/2021](https://commons.laws.africa/akn/rw/act/reg/2021/2/media/publication/rw-act-reg-2021-2-publication-document.pdf)). The research is thinner than for Nigeria, because most primary texts were unreadable and the notes rely on summaries: no numeric Rwandan STR deadline, no Rwandan cash or wire reporting threshold, no Bank of Ghana 2026 cyber directive incident clock and no Rwandan central bank incident clock were found, so those rules ship with tenant-configurable placeholders and are marked "threshold to be confirmed". 15 of the 132 rules are therefore off by default. Treat every [Regulatory] row as "confirm against the Gazette text before launch", and do not read this as legal advice.

## 1. How to read this library

**Scope.** This report covers transaction monitoring and cybersecurity presets for a multi-country RiskOS tenant operating in Ghana and Rwanda, as of 4 October 2026. It builds on the Nigeria library in `PROMPT_3_NIGERIA_RULES_AND_COMPLIANCE_DASHBOARD.md` (appendix sections 2 to 9) and does not rewrite it. Nigerian rules are referred to by their IDs (REG, ATO, VEL, MUL, BEH, CNP, LEN, SEC, GOV, INS, SIM, PON) and the crypto rules CRY-05 to CRY-22 are cross-referenced from `reports/Crypto compliance on and off chain.md` for virtual-asset activity. The brief mentioned twelve source notes; the folder held nine (ghana_aml, ghana_cyber, ghana_typologies, ghana_gap, rwanda_aml, rwanda_cyber, rwanda_typologies, rwanda_gap_thresholds, rwanda_cyber_gap), and all nine were read.

**Status labels** follow the Nigeria library.

| Status | Meaning | Where it appears |
|---|---|---|
| Regulatory | Required by a Ghanaian or Rwandan law, regulation or regulator directive. Encode as a hard control or legal clock, not a weighted score | REG rules and the legal-clock rules in SEC and GOV |
| Recommended | Common fraud or money-laundering typology. Starting weights and thresholds are defaults to tune on real data | TM rules and most SEC rules |
| Governance | Process requirements that shape alerts, cases, reporting and oversight | GOV rules |

**Tag conventions.** Each rule carries one provenance tag, matching the crypto report.

| Tag | Meaning |
|---|---|
| [Regulatory] | Follows a cited law, regulation or directive. Many rest on secondary summaries, which the basis column states |
| [Industry practice] | A published typology or practice (FATF-style, regulator fraud reports, press-reported cases) with no mandated number |
| [My design] | My proposal or an inference; no source states it. Numbers are placeholders to calibrate in shadow mode |

Each rule also has a lineage ("Param of ATO-01" means it is a country-parameterised version of a Nigerian rule; "New local" means it has no Nigerian counterpart) and a default state ("On", "Off (threshold to be confirmed)" or "Off (verify first)"). Where a threshold was not found, the rule ships with a tenant-configurable placeholder, is marked "threshold to be confirmed" and is disabled by default. Money is written in GHS (stored as pesewas) or RWF (whole francs); every amount is a tenant-configurable default; windows use Africa/Accra and Africa/Kigali time.

**Rule counts.**

| Country | REG (mandatory) | TM (typology and monitoring) | SEC (cybersecurity) | GOV (governance) | Total | [Regulatory] | [Industry practice] | [My design] | Off by default |
|---|---|---|---|---|---|---|---|---|---|
| Ghana | 13 | 28 | 14 | 13 | 68 | 30 | 28 | 10 | 7 |
| Rwanda | 14 | 24 | 16 | 10 | 64 | 31 | 27 | 6 | 8 |
| Both | 27 | 52 | 30 | 23 | 132 | 61 | 55 | 16 | 15 |

Of the 68 Ghana rules, 40 are country-parameterised versions of Nigerian rules (or analogues and cross-references) and 28 are new local rules; of the 64 Rwanda rules, 39 are parameterised versions and 25 are new local rules. Rules off by default: Ghana: GH-REG-13, GH-TM-16, GH-TM-19, GH-TM-23, GH-TM-24, GH-SEC-13, GH-GOV-11. Rwanda: RW-REG-05, RW-REG-08, RW-REG-14, RW-TM-16, RW-TM-18, RW-SEC-03, RW-SEC-16, RW-GOV-07.

**Inherited from Nigeria without a new ID.** These Nigerian rules apply in both countries by setting the country, time zone and amount floor on the tenant, because nothing in the notes changes their logic: ATO-04, ATO-05, ATO-06, VEL-03, VEL-05, VEL-06, BEH-01 (Ghana only; Rwanda uses it inside RW-TM-14), BEH-03, CNP-01 to CNP-03, LEN-01, LEN-03, LEN-04, SEC-02, SEC-03, SEC-05, SEC-06, SEC-07, SEC-11, GOV-01 to GOV-04 and GOV-07. For Rwanda, no digital-lending evidence was found, so the LEN rules stay available but unsupported by a local source.

**Top caveats.**

1. **Primary texts were mostly unreadable.** Fetches returned model summaries of statutes, and several Ghanaian and Rwandan regulator sites were blocked. Article and section numbers are "per summary" and need checking against the Gazette before hard-coding.
2. **Thin evidence is labelled, not filled.** Rwandan STR deadline, Rwandan cash and wire threshold, Bank of Ghana CISD 2026 incident clock, Medium and Enhanced wallet monthly caps, Travel Rule thresholds for both countries, and agent-float numbers were not found. Placeholders are marked.
3. **Conflicts exist and are listed in section 7.** The largest are Ghana CTR GHS 50,000 versus a vendor claim of GHS 10,000, Rwandan retention 5 versus 10 years, and Act 1154 commencement.
4. **Secondary-only sources** carry several headline numbers: the eKash RWF 10 million cap and RWF 20 fee (news aggregator), the GHS 12 penalty unit (a tax authority page), the Critical Information Infrastructure 24-hour and 72-hour clocks (aggregators), and the Rwandan draft SIM regulation (draft, status unknown).
5. **Rule logic is my design.** The WHEN/IF/THEN syntax is the Nigeria library's own notation, not a regulator format, and none of these rules has been backtested on Ghanaian or Rwandan data.
6. **Applicability depends on licence type.** Capture each tenant's type (bank, specialised deposit-taking institution, forex bureau, e-money issuer, payment service provider, virtual-asset provider, digital lender, telecom) so the right pack turns on.

## 2. Ghana regulatory picture

Ghana's AML framework rests on **Act 1044** (the Anti-Money Laundering Act 2020, 65 sections), whose s.38 requires suspicious transaction reports "within twenty-four hours" of forming knowledge or suspicion, and whose s.32 sets five-year record keeping ([GhaLII](https://ghalii.org/akn/gh/act/2020/1044/eng@2020-12-29), per summary). The Financial Intelligence Centre publishes the operative amounts: a **GHS 50,000** cash threshold for banking and securities, a **US$1,000** electronic currency transaction threshold for commercial banks, and a currency declaration report with no threshold ([FIC](https://fic.gov.gh/report-types.html)). The Bank of Ghana's revised AML/CFT&P Guideline (December 2022) adds **GHS 20,000** for specialised deposit-taking institutions and a 24-hour clock for cross-border reports ([BoG guideline](https://www.moneylaundering.com/wp-content/uploads/2022/12/Ghana.Guidance.AMLCTFGUIDELINe.122322.pdf)). Mobile-money tier limits changed on 1 March 2024 and no later change was found ([Graphic](https://www.graphic.com.gh/business/business-news/see-the-new-momo-balance-transaction-limits-effective-march-1.html)). On the payments and cyber side, **Act 987** gives the firmest verifiable obligations (10-day reporting, 6-year audit logs, outsourcing notice), while the Bank of Ghana launched a revised cyber directive on 30 March 2026 whose deadlines could not be retrieved ([GhaLII Act 987](https://ghalii.org/akn/gh/act/2019/987); [AllAfrica](https://allafrica.com/stories/202603310611.html)). Ghana left the FATF grey list on 25 June 2021 and the GIABA onsite visit ran 26 January to 6 February 2026, with the report pending ([FIC](https://fic.gov.gh/event-ghana-third-round-mutual-evaluation.html)).

### 2.1 Regulators and laws

| Instrument | Regulator or body | What it governs | Status and notes | Source |
|---|---|---|---|---|
| Anti-Money Laundering Act 2020 (Act 1044) | Financial Intelligence Centre (FIC); supervisors include Bank of Ghana (BoG) and SEC | Master AML statute: CDD, records (s.32), STR (s.38), cash and electronic transfer reports (ss.40-42), suspension (s.45), freezing (s.56), PEPs (s.49), penalties (s.48) | In force; no amendment found | GS3, GS4 |
| AML Regulations 2011 (L.I. 1987) | FIC | Supporting regulations referenced in the SEC sanctions schedule | Contents not retrieved | GS5 |
| BoG AML/CFT&P Guideline (December 2022) | BoG | Banks and specialised deposit-taking institutions: CTR, cross-border reports, STR, PEPs, review cycle, testing, training | In force from issue; vendor claim of a January 2026 update unverified | GS2, GS11 |
| Forex bureaux AML guideline (effective September 2025) | BoG | Ghana Card, enhanced due diligence at US$10,000, CTR above GHS 20,000, calendar | In force | GS9 |
| SEC administrative sanctions schedule (effective 1 January 2022) | SEC | 31 breach categories and penalty-unit ranges | In force | GS5 |
| Executive Instrument 2 (UN Security Council implementation) | FIC; Minister for Foreign Affairs | Freezing without prior notice, 14-day petition, quarterly reports and drills | Per summary only | GS7 |
| Anti-Terrorism Act 2008 (Act 762), amended by Act 875; Proceeds of Crime Act 2009 (Act 759) | FIC, courts | Terrorism financing offences, proceeds of crime | Texts listed, not read | GS7, GS38 |
| Payment Systems and Services Act 2019 (Act 987) | BoG | Payment institutions: fraud monitoring, audit logs, reporting, outsourcing, CDD, licensing | In force; section numbers per summary | GS24 |
| E-money Issuer and Agent Guidelines (notice BG/GOV/SEC/2015/09) | BoG | Wallet and agent rules; dedicated e-money issuer licence category | Existence only; text not retrieved | GS42 |
| Directive for Digital Credit Service Providers (September 2025) | BoG | Digital lender licensing and conduct; 20 lenders named on 3 August 2026 | In force | GS32 |
| Virtual Asset Service Providers Act 2025 (Act 1154) | BoG, SEC, FIC; Virtual Assets Coordinating Committee | Registration, licensing or waiver; compliance with Act 1044; sandbox | Passed December 2025; commencement and full operation (2027) unresolved | GS12-GS17 |
| Cybersecurity Act 2020 (Act 1038) | Cyber Security Authority (CSA) | Critical information infrastructure, 24-hour incident reporting, licensing of providers | In force since 29 March 2021; Amendment Bill consulted to 14 November 2025, status unknown | GS18-GS21 |
| BoG Cyber and Information Security Directive (2018; revised version launched 30 March 2026) | BoG; FICSOC as sectoral CERT | Governance, cloud, AI, board expertise, proportionality | 2026 text and deadlines not found | GS22, GS23, GS25 |
| Data Protection Act 2012 (Act 843) | Data Protection Commission (DPC) | Registration (2-year renewal), breach notice, transfers | In force; Data Protection Bill 2025 status unconfirmed | GS26 |
| Electronic Communications and SIM re-registration (NCA, 2022) | National Communications Authority | SIM linked to Ghana Card, deactivation sanction | Act 772 and Act 775 provisions not found | GS27 |
| Fines (Penalty Units) Act 2000 (Act 572) | Parliament | Penalty unit value | GHS 12 per a GRA page, effective date unstated | GS6 |

### 2.2 Thresholds and deadlines

| Item | Value | Applies to | Source |
|---|---|---|---|
| Cash transaction report (CTR) | GHS 50,000 (banking, securities); GHS 20,000 (specialised deposit-taking, forex bureaux). ">=" assumed | Cash | GS1, GS2, GS9 |
| Electronic currency transaction report (ECTR) | US$1,000 or cedi equivalent, inward and outward; within 24 hours after the transfer | Cross-border transfers | GS1, GS2 (s.2.9.2) |
| Suspicious transaction report | 24 hours from forming suspicion (Act); 24 hours from confirmation (BoG); no minimum amount | All accountable institutions | GS2 (s.2.8.2), GS3 (s.38) |
| Currency declaration report | No threshold | Entry points | GS1 |
| Record retention | 5 years: ID files after relationship ends, transactions from the transaction, reports from report date | Act 1044 s.32 | GS3 |
| Audit log retention | 6 years, complete log of user activity | Payment institutions (Act 987 ss.20, 34) | GS24 |
| PEP and customer review | Senior management approval; existing customers reviewed every 2 years | BoG guideline | GS2 (ss.2.7.1, 2.4.5) |
| AML system testing; staff training | At most yearly testing; at least twice-yearly training | BoG guideline | GS2 (ss.2.10, 2.3.3) |
| Wallet daily limit (from 1 March 2024) | Minimum GHS 3,000; Medium GHS 15,000; Enhanced GHS 25,000 | Mobile money | GS8 |
| Wallet maximum balance | Minimum GHS 5,000; Medium GHS 40,000; Enhanced GHS 75,000 | Mobile money | GS8 |
| Wallet monthly limit | Minimum GHS 10,000; Medium and Enhanced not confirmed | Mobile money | GS8 |
| Forex bureau EDD and biometrics | US$10,000 or more | Forex bureaux | GS9 |
| Forex bureau calendar | Annual audit by 30 April; training submission by 31 December; remediation within 45 days; STR in 24 hours if sanctioned persons identified | Forex bureaux | GS9 |
| FIC suspension and freeze | Suspension up to 7 working days; freeze by FIC chief executive, court application within 7 working days (E.I. 2 summary says 48 hours) | Accounts and transactions | GS3 (ss.45, 56), GS7 |
| Report fraud, breach, service interruption, data loss to BoG | Within 10 days | Payment institutions (Act 987 s.42(5)) | GS24 |
| Outsourcing | Written notice before outsourcing; SLA submitted within 10 days of signing | Payment institutions (Act 987 s.38) | GS24 |
| CSA incident and vulnerability clocks | Incident within 24 hours of detection; vulnerability within 72 hours | Designated critical information infrastructure owners | GS18, GS19 |
| CSA licensing enforcement | From 31 January 2026 | Cybersecurity providers (Act 1038 s.49(1)) | GS20 |
| DPC registration; breach notice | Renew every 2 years; breach notice "as soon as reasonably practicable" (no hour count) | Data controllers | GS26 |
| Payment institution licence evaluation | 90 days | Applicants (Act 987 ss.7-14) | GS24 |
| VASP registration renewal; BoG application pages | 12 months (Bill, clause 13); pages due end of Q3 2026 | Virtual-asset providers | GS13, GS17 |
| Beneficial ownership | 25% (Companies Act 2019, vendor-reported) | Companies | GS11 |

### 2.3 Penalties

Penalty-unit ranges are from the notes; cedi conversions use GHS 12 per unit and are my inference, because the GRA page gives no effective date.

| Provision | Penalty | Cedi estimate (inference) | Source |
|---|---|---|---|
| Act 1044 s.48(1): failure to report in time, tipping off (s.38(4)) | 500 to 4,000 penalty units and/or 6 months to 5 years | GHS 6,000 to 48,000 | GS3 |
| Act 1044 s.48(2): false statements, concealment | 1,000 to 10,000 penalty units and/or 12 months to 5 years | GHS 12,000 to 120,000 | GS3 |
| Act 1044 s.48(5): employee non-compliance | Up to 10,000 penalty units | Up to GHS 120,000 | GS4 |
| SEC schedule | Institutions 1,000 to 100,000 units; individuals 500 to 20,000 units; blacklisting, licence suspension, public disclosure | GHS 12,000 to 1,200,000 (institutions) | GS5 |
| Act 987 s.42(3): late reports | 500 units plus 50 units per day | GHS 6,000 plus GHS 600 per day | GS24 |
| Act 987 s.35: CDD non-compliance | 2,000 units | GHS 24,000 | GS24 |
| Act 987 s.9: unlicensed operation (body corporate) | 4,500 to 7,000 units | GHS 54,000 to 84,000 | GS24 |
| Act 1038: hacking | Up to 10 years; directors liable unless without knowledge or consent | Not stated | GS19 |
| Act 843 | Conflicting: up to 150 units or 1 year (summary offence), 5,000 units or 10 years (others) per Mondaq; GHS 1,500 and GHS 60,000 per Templars | Conflict: 150 x 12 = GHS 1,800 versus GHS 1,500 | GS26 |
| Vendor claim: up to GHS 500,000 per violation | Conflicts with the penalty-unit regime | Do not use | GS11 |

Ghana's fraud picture shapes the typology rules. The Bank of Ghana's 2025 fraud report counted 24,778 cases (up 48% year on year) with PSP cases up 98%, bank cash-suppression value at risk of GH¢40.7m driven by one GH¢36m case, and about 139 of 219 staff fraud cases involving cash theft or suppression ([Citi Newsroom](https://www.citinewsroom.com/2026/07/bog-2025-fraud-report-the-numbers-beneath-the-headline/); [MyJoyOnline](https://www.myjoyonline.com/bog-flags-gh%C2%A240-7m-cash-suppression-risk-after-single-gh%C2%A236-million-fraud-case/)). Mobile money moved GH¢518.4bn across 982m transactions in December 2025, an average of about GHS 528 (computed), which is why the default floors are low ([Graphic](https://www.graphic.com.gh/business/business-news/mobile-money-transactions-reach-ghc518b-in-2025.html)). The FIC's personal-account tax typology and a gold-trade gap estimated at 229 tonnes (US$11.4bn, 2019 to 2023) inform GH-TM-18 and GH-TM-19, though no FIC galamsey red-flag list exists in the notes ([FIC](https://fic.gov.gh/media/attachments/2026/01/22/suspicious-use-of-personal-bank-accounts-for-business-transactions-to-evade-payment-of-tax.pdf); [Mining Index](https://www.miningindex.co.zw/?p=109878)). E-Levy was repealed (Parliament approved the repeal on 26 March 2025), so the library deliberately has no levy-avoidance rule ([GNA](https://gna.org.gh/2025/03/parliament-abolishes-e-levy-returns-gh2-billion-to-ghanaians/)).

## 3. Rwanda regulatory picture

Rwanda's AML law is **Law No. 001/2025 of 22 January 2025**, which superseded Law 028/2023 (itself the successor to Law 75/2019), so every rule cites the 2025 law and article numbers from 2023 may have moved ([RegAlert](https://regalert.today/topics/aml/RW); [ESAAMLG FUR](https://www.esaamlg.org/reports/Rwanda%20FUR-August%202025.pdf)). The 2025 law defines "immediately" as within **24 hours** for freezing under Art 2(b) and Art 4, and the ESAAMLG follow-up report cites a **10-year** retention duty in Art 21 ([RegAlert Law 001/2025](https://regalert.today/document/195f62e1-4ddf-4323-86ae-c8a19628da31/text); [ESAAMLG](https://www.esaamlg.org/reports/Rwanda%20FUR-August%202025.pdf)). The numeric STR deadline (Art 32) and any cash or wire reporting amount were not found in any readable source. Cross-border declaration is **US$10,000** under Regulation 002/FIC/2021 Art 4 ([Laws.Africa](https://commons.laws.africa/akn/rw/act/reg/2021/2/media/publication/rw-act-reg-2021-2-publication-document.pdf)). Rwanda enacted a virtual-asset law, **Law 023/2026**, effective 28 May 2026, with the Capital Market Authority as primary regulator and the central bank as secondary ([Andersen](https://rw.andersen.com/dist/virtual-assets.html); [PwC](https://www.pwc.com/rw/en/assets/pdf/rwandas-new-virtual-law.pdf)). The 2024 mutual evaluation found low effectiveness on 10 of 11 immediate outcomes and the August 2025 follow-up kept Rwanda in enhanced follow-up; no grey-list listing was found, but it was not confirmed either ([FATF](https://www.fatf-gafi.org/content/fatf-gafi/en/publications/Mutualevaluations/MER-Rwanda-2024.html)).

### 3.1 Regulators and laws

| Instrument | Regulator or body | What it governs | Status and notes | Source |
|---|---|---|---|---|
| Law No. 001/2025 of 22/01/2025 (ML, TF, PF) | FIC | Master AML law: freezing (Art 2(b), 4, 6), records (Art 21), PEPs (Art 22), STR (Art 32), non-disclosure (Art 34), offences (Arts 56-66) | In force; article map from an aggregator | RS1, RS3 |
| Law 028/2023 (and Law 75/2019, Law 74/2019) | FIC | Predecessor regimes | Superseded; 028/2023 Art 31 required cash and wire reports | RS1, RS4, RS8 |
| FIC Law amendment (Law 045/2021, amended January 2025) | FIC | Higher fines, wider inspection powers | No amounts disclosed | RS7 |
| FIC Regulations 001/FIC/2025 (15 January 2025) | FIC | Administrative sanctions for targeted financial sanctions | In force | RS2 |
| FIC Regulation 002/FIC/2021 (26 August 2021) | FIC, customs | Cross-border cash declaration | Not verified that Law 001/2025 left it in force | RS5 |
| FIC Guidelines 001/2023 (21 November 2023) | FIC | Beneficial ownership transparency | Listed only | RS32 |
| BNR Regulation 72/2023; AML-CFT General Guidelines 4230/2023-00076; CMA Guidelines (31 March 2023) | National Bank of Rwanda (BNR); Capital Market Authority (CMA) | Administrative sanctions and sector AML guidance | Listed as in force | RS1 |
| Law 061/2021 (payment system, 14 October 2021) | BNR | Licensing of payment service providers | In force 1 November 2021 | RS19 |
| BNR Regulations 54/2022 (e-money), 74/2023 (PSPs), 77/2023, 73/2023 (agents), 64/2023 (remittance), 78/2023 (sanctions), 79/2023 (users) | BNR | E-money (Art 9 identity and three accounts, Art 10 authentication, Art 36 issuer-defined limits), PSP cyber (Art 54), outsourcing (Art 44) | In force | RS10, RS11, RS19 |
| BNR Directive 45/2026 | BNR; RSwitch operates eKash | Interoperable P2P through eKash from 14 July 2026; RWF 10 million cap; RWF 20 fee ceiling | News aggregator only; directive dated 9 January 2026 per RBA | RS12 |
| BNR Directive 02/2009 | BNR | RWF 5 million daily cash withdrawal ceiling | 2009 text; verify it is still current | RS9 |
| Law No. 023/2026 (virtual assets, 25/05/2026) | CMA primary; BNR secondary | Licensing, AML controls, Travel Rule reference, penalties | Effective 28 May 2026; implementing regulations pending | RS13 |
| BNR Regulation 50/2022 (cyber security in regulated institutions, 2 June 2022) and Directive 4230/2024-00037 (incident management, 17 January 2024) | BNR | Governance, pentest cadence, MFA, incident reporting (Art 21) | Art 21 hour count not found | RS17, RS18 |
| Law 058/2021 (data protection, 13 October 2021) | National Cyber Security Authority (NCSA) | Registration, 48-hour breach notice, localisation, transfers, DPO | In force 15 October 2021; article numbers from one secondary source | RS14 |
| Law 60/2018 (cybercrime) | NCSA, RIB | Offences; provider reporting and evidence preservation (Arts 5-6) | In force | RS15 |
| NCSA minimum standards (28 July 2023; implementation 28 July 2024); National Cybersecurity Strategy 2024-2029 | NCSA; Rw-CSIRT | Essential service providers; national strategy | Banks and PSPs covered only if designated; unconfirmed | RS16 |
| RURA Telecom Network Security Regulations (2016) | Rwanda Utilities Regulatory Authority (RURA) | Incident reporting, testing cadence, audits for telecom licensees | In force | RS20 |
| RURA SIM registration (NIDA and face verification from 1 September 2021; draft regulation January 2023) | RURA | SIM registration, replacement, caps | Draft status unknown | RS21 |
| BNR Regulation 94/2026; BNR AI risk guidelines (June 2026) | BNR | PSP acquisitions; AI risk | Listed only | RS1, RS17 |

### 3.2 Thresholds and deadlines

| Item | Value | Applies to | Source |
|---|---|---|---|
| Freeze deadline | "Immediately" means within 24 hours at the latest (Art 2(b), Art 4); 2023 law allowed up to 3 working days | Targeted financial sanctions | RS3, RS4 |
| STR deadline | Not found; Art 32 duty only | All reporting entities | RS3, RS6 |
| Cash or wire report threshold | Not found | Predecessor Art 31 | RS4 |
| Cross-border cash declaration | Above US$10,000 or equivalent; authority informs FIC within 30 minutes of a suspicious incident, holds cash up to 2 hours, sends declarations within 2 days | Travellers | RS5 |
| Record retention | At least 10 years after the relationship ends (Art 21 per FUR); 5 years in the 2024 MER under the old law | Reporting entities | RS2, RS6 |
| e-money limits | Issuer-defined (Art 36); no BNR wallet cap found. Maximum 3 e-money accounts per customer without BNR authorisation (Art 9) | E-money issuers | RS11 |
| eKash | RWF 10 million per transaction cap; fee ceiling RWF 20; live 14 July 2026 | Interoperable P2P | RS12 |
| Cash withdrawal | RWF 5 million per day through any instrument; exceptions reported weekly | Banks (2009 directive) | RS9 |
| BNR pentest and assessment | Annual penetration test; vulnerability assessment twice a year; executive summary to BNR within 15 days of the test | BNR-regulated institutions (Reg 50/2022 Art 10) | RS17 |
| MFA | Mandatory for remote access to internal networks unless the IT security head approves an equivalent control in writing (Art 16) | BNR-regulated institutions | RS17 |
| Personal-data breach | 48 hours to NCSA from awareness (Art 43); data subjects when risk is high (Art 45) | Controllers and processors | RS14 |
| Registration certificate | Issued within 30 days (Art 31); personal data stored in Rwanda by default (Art 50); transfer needs NCSA authorisation, consent or necessity, plus a contract (Arts 48-49) | Controllers and processors | RS14 |
| Telecom incidents | Critical and major reported immediately; moderate monthly (Art 28); annual pentest; vulnerability assessment twice a year; audits twice a year; remediation within 3 months; reports within 30 days | Telecom licensees | RS20 |
| SIM rules (draft) | NIDA biometric check; failed fingerprints cleared within 48 hours; up to 3 SIMs per operator for Rwandan ID holders; illegal incidents reported within 48 hours | Telecom operators | RS21 |
| PSP licence recategorisation | By 18 September 2024 (headline only) | PSPs under Reg 74/2023 | RS30 |
| NCSA incident channels | incident@ncsa.gov.rw, toll-free 9009, Rw-CSIRT at cert.gov.rw; no hour count | All | RS22 |

### 3.3 Penalties

| Provision | Penalty | Source |
|---|---|---|
| Targeted financial sanctions administrative fine (FIC Reg 001/FIC/2025 Arts 26-27) | RWF 1,000,000 to RWF 10,000,000 | RS2 |
| Terrorism financing (Art 57(1)); non-compliance with preventive measures | 20 to 25 years; 1 to 3 years | RS2 |
| Earlier regime (Laws 74 and 75/2019) | Up to 25 years; fines up to 20 times the financed illegal activity (superseded) | RS8 |
| Law 058/2021 administrative (Art 53) | RWF 2,000,000 to RWF 5,000,000 or 1% of global turnover; criminal up to 5% of annual turnover (Art 62); up to 10 years; cancellation of registration. ConsentStack lists a RWF 5,000,000 maximum: conflict | RS14 |
| Law 60/2018 | Unauthorised access 6 months to 2 years and RWF 1 to 2 million; malware 2 to 5 years; critical infrastructure 10 to 15 years and RWF 10 to 30 million | RS15 |
| Law 023/2026 | Unlicensed natural person RWF 30 to 50 million and 3 to 5 years; legal entity RWF 70 to 100 million; unlicensed issuance RWF 120 to 150 million; unapproved marketing RWF 15 to 20 million | RS13 |
| RURA Telecom Network Security Regulations | RWF 1 to 5 million; RWF 500,000 per day for delayed compliance; RWF 500,000 to 1 million for late incident information; RWF 200,000 to 1 million for late audit submissions | RS20 |
| RURA SIM draft | RWF 10,000,000 for illegal SIM swaps; RWF 2,500,000 per SIM for unauthorised use of subscriber data | RS21 |
| Foreign-currency pricing | RWF 5 million first offence; RWF 10 million repeat | RS27 |
| BNR Reg 50/2022 Art 25; Reg 54/2022 Art 44 | Amounts not found | RS17, RS11 |

The loss data is thin. The central bank said digital-payment fraud cases fell from about 5,000 to about 3,000 in 2025, with mobile money the leading channel, though one headline says 60% and the figures imply 40% ([AllAfrica](https://allafrica.com/stories/202602200031.html)). A survey of lost-money respondents found mule scams 29%, account takeover 24%, social engineering 22%, and fraud concentrated at account creation (7.7%) rather than login (1.6%) or transactions (0.5%), with a median loss of RWF 869,249 ([New Times](https://www.newtimes.co.rw/article/36384/news/technology/digital-fraud-in-rwanda-declines-below-global-average/amp)); the survey's vendor and method are unidentified. The 2024 national risk assessment rates overall ML risk Medium, real estate Medium-High, and attributes proceeds to embezzlement (39%), fraud (24%), tax crimes (20%) and cybercrime (13%) ([KT Press](https://www.ktpress.rw/2025/07/how-rwf-93-billion-in-criminal-proceeds-flowed-through-rwandas-economy/); [NRA](https://icparwanda.com/wp-content/uploads/2025/05/2024-NRA-Report.pdf)). The March 2026 Equity Bank Rwanda case (about US$3.4m, 35 arrests) exploited float mechanisms and unregistered or foreign-linked SIMs ([ITWeb Africa](https://itweb.africa/article/cyber-fraud-storm-hits-equity-bank-rwanda-as-35-arrested/lwrKx73YR81qmg1o)).

## 4. Ghana preset rules

Notation is identical to the Nigeria library (section 7B): `RULE id | weight or control | action`, `WHEN`, `SCOPE`, `IF` / `AND` / `ANY OF`, `WINDOW ... BY ...`, `THEN`. Hard controls (REG, and the clock and control rules in SEC and GOV) set a decision directly; typology rules only add signal weight, and the decision policy decides. Money is GHS stored as pesewas; windows use Africa/Accra. Source keys (GS1 and so on) are listed in section 7. "Param of X" means a country-parameterised version of Nigerian rule X.

### 4.1 Plain-English tables

#### Mandatory controls (GH-REG)

| ID | Rule | What we look for (plain English) | Action or points | Tag, lineage, default, basis |
|---|---|---|---|---|
| GH-REG-01 | Wallet daily limit by KYC tier | A mobile-money wallet moves more in a day than its KYC tier allows: Minimum GHS 3,000, Medium GHS 15,000, Enhanced GHS 25,000 (limits effective 1 March 2024). | Hard: decline the transaction that would breach the limit | [Regulatory]. New local (Nigeria keeps tier limits as per-institution config). Default: On. Basis: GS8, GS10 (conflict note) |
| GH-REG-02 | Wallet maximum balance by KYC tier | A credit would push a wallet above the maximum balance for its tier: Minimum GHS 5,000, Medium GHS 40,000, Enhanced GHS 75,000. | Hard: reject or hold the excess credit | [Regulatory]. New local. Default: On. Basis: GS8 |
| GH-REG-03 | Wallet monthly limit by KYC tier | A Minimum-tier wallet moves more than GHS 10,000 in a month. Medium and Enhanced monthly caps could not be confirmed, so they are blank until the tenant enters them. | Hard for Minimum tier; Medium and Enhanced skipped until configured | [Regulatory]. New local. Default: On. Basis: GS8; notes conflict (see section 7) |
| GH-REG-04 | Cash transaction report (CTR) | A cash deposit, withdrawal or payment reaches GHS 50,000 at a bank or securities firm, or GHS 20,000 at a specialised deposit-taking institution or forex bureau. | Report: prepare a CTR for the FIC; due within 24 hours of the transaction (configurable) | [Regulatory]. Param of REG-17 (different amounts and scope). Default: On. Basis: GS1, GS2, GS3, GS9; vendor conflict GS11 |
| GH-REG-05 | Electronic transfer report (cross-border) | A cross-border transfer, in or out, of US$1,000 or more (or the cedi equivalent). Not US$10,000. | Report: prepare an electronic currency transaction report for the FIC within 24 hours of the transfer | [Regulatory]. Param of REG-10 (threshold USD 1,000 instead of USD 10,000). Default: On. Basis: GS1, GS2 (s.2.9.2), GS3 (ss.40-42) |
| GH-REG-06 | Suspicious transaction report (STR) | A staff member or the system forms a suspicion. The STR is due within 24 hours, with no minimum amount. Telling the customer is a crime. | Report: 24-hour filing clock to the FIC; block every customer-facing signal of the report | [Regulatory]. Param of REG-14 (24h clock; no 72h screening stage found in Ghana sources). Default: On. Basis: GS2 (s.2.8.2), GS3 (s.38) |
| GH-REG-07 | Politically exposed person (PEP) | The customer or an owner behind a company is a PEP. Senior management must approve the relationship; source of wealth is verified; PEP transactions are reported to the FIC. | Hard: enhanced due diligence and senior approval; add transactions to the PEP report | [Regulatory]. Param of REG-12 and REG-18. Default: On. Basis: GS2 (s.2.7.1, s.2.4.5), GS4 (s.49(2)(a)(ii)) |
| GH-REG-08 | Sanctions and UN freeze | A name, ID or account matches the UN Security Council lists, Ghana designations or other configured lists. Freeze without prior notice and tell the FIC. | Hard: block, freeze, open case, report; freeze service level is a placeholder | [Regulatory]. Param of REG-11 (freeze clock placeholder). Default: On. Basis: GS7 (E.I. 2), GS3 (s.56) |
| GH-REG-09 | Ghana Card identity at onboarding | A customer is opened without a verified Ghana Card. Mandatory for forex bureaux; other tenants switch it on if their licence guideline requires it. | Hard for forex bureaux; tenant toggle for others: block the account action | [Regulatory]. Analogue of REG-06 and REG-16 (Ghana Card instead of BVN or NIN). Default: On. Basis: GS9; GS24 (s.35, ID types) |
| GH-REG-10 | No anonymous or fictitious accounts | An account is opened or kept without a verified, real identity behind it. | Hard: block opening; restrict and review existing accounts | [Regulatory]. New local. Default: On. Basis: GS4 (s.48(1)(a)); GS3 |
| GH-REG-11 | FIC suspension and freeze orders | The FIC suspends a transaction (up to 7 working days) or freezes an account. The institution applies the hold, lets it expire on the right date and does not tip off the customer. | Hard: hold with automatic expiry and audit entry | [Regulatory]. Analogue of the 72h NFIU stop order (Nigeria section 8). Default: On. Basis: GS3 (ss.45, 56, per summary) |
| GH-REG-12 | Forex bureau high-value checks | A forex bureau customer buys or sells US$10,000 or more. Enhanced due diligence and biometric verification are required. Cash above GHS 20,000 also triggers the CTR in GH-REG-04. | Hard (forex bureaux only): require EDD and biometrics | [Regulatory]. New local. Default: On. Basis: GS9 |
| GH-REG-13 | Virtual-asset Travel Rule data | A virtual-asset transfer above the Travel Rule limit lacks sender or receiver details. The Ghana limit and data fields are not yet published, so the rule ships off. | Hard when enabled: hold crediting or release | [Industry practice]. Cross-reference CRY-13 (threshold not found). Default: Off (threshold to be confirmed). Basis: GS12, GS13, GS14, GS17 (secondary) |

#### Typology and monitoring rules (GH-TM)

| ID | Rule | What we look for (plain English) | Action or points | Tag, lineage, default, basis |
|---|---|---|---|---|
| GH-TM-01 | New device, new payee, then large transfer | Within 30 minutes: a new device or app is linked, a new payee is added, and money is sent above the institution's minimum. Ghana has no found cap on new-device transfers like Nigeria's REG-01, so this only scores. | 30 points, review | [Industry practice]. Param of ATO-01 (no Ghana equivalent of REG-01 found). Default: On. Basis: GS23, GS29 (modus operandi) |
| GH-TM-02 | SIM swap, then transfer or cash-out | The telco reports a SIM swap, or the SIM or device was re-registered in the last 72 hours, then money is sent or cashed out. | 25 points, step-up | [Industry practice]. Param of ATO-03. Default: On. Basis: GS23, GS35 |
| GH-TM-03 | Credentials changed, then electronic transfer | Password, PIN, phone or email changed in the last 24 hours, then an electronic transfer above the minimum. E-transfers were half of all bank fraud cases in the GAB sample. | 25 points, step-up | [Industry practice]. Param of ATO-02. Default: On. Basis: GS29 |
| GH-TM-04 | Impossible travel or new country | A login or payment from a country the customer does not normally use (10 points), or two events too far apart to travel between in the time (20 points). | 10 or 20 points | [Industry practice]. Param of BEH-02. Default: On. Basis: Nigeria library BEH-02 |
| GH-TM-05 | Sudden spike against the customer's own pattern | Money sent in 24 hours is more than three times the customer's usual daily amount and above the institution's minimum. | 20 points | [Industry practice]. Param of VEL-02. Default: On. Basis: GS34 (average ticket) |
| GH-TM-06 | Burst of transfers | Five or more transfers out in 10 minutes. | 15 points | [Industry practice]. Param of VEL-01. Default: On. Basis: Nigeria library VEL-01 |
| GH-TM-07 | Splitting to stay under wallet-tier limits | Three or more transactions in 24 hours each just under the wallet's daily limit, or the wallet uses 80% or more of its daily limit on three days in a week. | 25 points, review | [My design]. New local (tier-limit variant of VEL-04). Default: On. Basis: GS8; inference only |
| GH-TM-08 | Splitting cash to stay under the CTR line | Three or more cash deposits or withdrawals within 5 days, each between 90% and 99.9% of the CTR amount, including across branches or linked accounts. | 25 points, review | [Industry practice]. Param of VEL-04 (5-day window, multi-branch aggregation). Default: On. Basis: GS30 (FIC typology), GS1 |
| GH-TM-09 | Many senders into one wallet | Ten or more different senders pay one wallet or account in 24 hours (prize, romance or investment collections). | 20 points | [Industry practice]. Param of MUL-02. Default: On. Basis: GS28, GS35 |
| GH-TM-10 | Money passing straight through | 80% or more of money received leaves within 60 minutes. | 25 points, review | [Industry practice]. Param of MUL-01. Default: On. Basis: GS28, GS34 |
| GH-TM-11 | One payment split many ways | One incoming payment is split among five or more recipients within 60 minutes. | 20 points | [Industry practice]. Param of MUL-03. Default: On. Basis: GS28 |
| GH-TM-12 | Quick cash-out after money arrives | Money arrives by card or peer-to-peer and 70% or more is withdrawn at an ATM, POS or agent within 30 minutes. | 15 points | [Industry practice]. Param of MUL-08 (agent cash-out added). Default: On. Basis: GS28 |
| GH-TM-13 | New account, big volume | A wallet or account under 7 days old moves more than ten times what is normal for its KYC tier. | 20 points | [Industry practice]. Param of MUL-04. Default: On. Basis: GS28 |
| GH-TM-14 | Dormant account wakes up | No activity for 90 days or more, then a large payment in or out. | 20 points | [Industry practice]. Param of MUL-05. Default: On. Basis: Nigeria library MUL-05 |
| GH-TM-15 | Shared Ghana Card, phone or device across wallets | One device is used on three or more accounts, or one Ghana Card number, phone number or address appears on several accounts or applications. | 25 points, review | [Industry practice]. Param of MUL-06 (Ghana Card number added). Default: On. Basis: GS28, GS29 |
| GH-TM-16 | Agent float anomaly | An agent's float top-ups or cash-in and cash-out cycling run far above similar agents, or many of the agent's customers are brand-new wallets. No Ghana float data was found, so every number is a placeholder. | 25 points, review | [My design]. New local. Default: Off (threshold to be confirmed). Basis: Nigeria library gap; GS34 (491,000 agents); no float source |
| GH-TM-17 | Insider cash suppression | A teller, relationship officer or sales agent records cash received but the deposit is not posted, end-of-day cash differs from the ledger, or staff debit dormant customer accounts. BoG reported GH¢40.7m cash-suppression risk in 2025. | 30 points, review (compliance only) | [Industry practice]. New local (extends INS-01). Default: On. Basis: GS28, GS29 |
| GH-TM-18 | Personal account used as a business account | A personal account shows business-like turnover: repeated transfers with business accounts, funds from a shareholder labelled as loans or borrowing, or heavy activity in relatives' accounts. These are FIC tax-evasion red flags. | 20 points | [Industry practice]. New local. Default: On. Basis: GS30 |
| GH-TM-19 | Gold-trade pattern (galamsey) | A personal or small-business account receives money from gold buyers, aggregators or forex bureaux and cashes out quickly. The FIC has published no galamsey red-flag list, so this needs a counterparty tag list and tenant values. | 25 points, review | [My design]. New local. Default: Off (threshold to be confirmed). Basis: GS31 (Swissaid, gold share of exports); no FIC list |
| GH-TM-20 | Payment to an unlicensed digital lender | A payment goes to, or a borrower receives money from, a lender on the BoG list of unlicensed digital credit providers (20 named on 3 August 2026). | 30 points, review | [My design]. New local (list-based). Default: On. Basis: GS32 |
| GH-TM-21 | Borrow and cash out | 90% or more of a loan is moved out within 30 minutes of payout, or the borrower sends it to several numbers. | 20 points | [Industry practice]. Param of LEN-02. Default: On. Basis: GS32 |
| GH-TM-22 | Ponzi-style pooling | Twenty or more unrelated senders pay one account in 7 days, and 30% or more of what it pays out goes to people who paid in earlier. Memos promising fixed or doubled returns add support. | 25 points, review | [Industry practice]. Param of PON-01. Default: On. Basis: GS33 |
| GH-TM-23 | Wrong-transfer scam | An unexpected credit from a first-time sender is followed within hours by a 'please send it back' payment to a different number. No Ghana loss figures were found, so this ships off. | 25 points, review | [My design]. New local. Default: Off (threshold to be confirmed). Basis: GS28, GS35 (social engineering); inference only |
| GH-TM-24 | Cross-network pass-through (interoperability) | A freshly funded wallet sends most of its money to another network through GhIPSS interoperability within an hour. Interoperable flows are small, so this is a layering hint, not a sourced typology. | 20 points | [My design]. New local. Default: Off (threshold to be confirmed). Basis: GS34; inference only |
| GH-TM-25 | Romance-scam proceeds passed on | A foreign transfer arrives and 70% or more is sent on to three or more Ghanaian accounts within 72 hours. Where the sender profile is known, an elderly overseas sender strengthens the signal. | 25 points, review | [Industry practice]. New local. Default: On. Basis: GS36 (EOCO, US DOJ) |
| GH-TM-26 | Splitting forex deals under the US$10,000 line | A forex bureau customer makes two or more deals within 7 days, each between 90% and 99.9% of US$10,000. | 25 points, review | [My design]. New local (derived from the forex EDD trigger). Default: On. Basis: GS9; derivation is my design |
| GH-TM-27 | Exposure to FATF-listed jurisdictions | A cross-border transfer involves a country on the FATF increased-monitoring list. The FIC asks for risk-based heightened scrutiny, not blanket enhanced due diligence. | 10 points | [Industry practice]. New local. Default: On. Basis: GS38 (FIC advisory, 25 Aug 2025) |
| GH-TM-28 | Mobile money to crypto off-ramp | Many senders fund one wallet and 70% or more of it goes to crypto exchanges or peer-to-peer crypto counterparties within 7 days. Cross-reference CRY-01 and CRY-08. | 25 points, review | [My design]. New local (cross-reference CRY-01, CRY-08). Default: On. Basis: GS12, GS15 (Act 1154 not yet operational) |

#### Cybersecurity rules (GH-SEC)

| ID | Rule | What we look for (plain English) | Action or points | Tag, lineage, default, basis |
|---|---|---|---|---|
| GH-SEC-01 | Stolen-password testing (credential stuffing) | Ten or more failed logins from one network address across five or more accounts in 5 minutes. | 30 points, block IP or challenge | [Industry practice]. Param of SEC-01. Default: On. Basis: GS23 |
| GH-SEC-02 | Approval-prompt flooding and OTP interception | Five or more push or OTP prompts to one user in 10 minutes, or an OTP used from a different device than the one that requested it. | 25 points, pause prompts | [Industry practice]. Param of SEC-04 (OTP interception clause added). Default: On. Basis: GS35 (OTP interception) |
| GH-SEC-03 | API key misuse | A key is used from a new network or country, at five times its normal rate, or with many failed or not-found requests. | 30 points, throttle | [Industry practice]. Param of SEC-08. Default: On. Basis: Nigeria library SEC-08 |
| GH-SEC-04 | Rights raised, then data exported | A user's role is upgraded and they export data within the hour. | 30 points, alert and step-up | [Industry practice]. Param of SEC-10. Default: On. Basis: GS26 (breach duty) |
| GH-SEC-05 | Unusually large export | An export far above the user's or institution's normal size. A large unexplained export may be a reportable personal-data breach. | 25 points, step-up and alert | [Industry practice]. Param of SEC-12. Default: On. Basis: GS26 |
| GH-SEC-06 | Payee or payout details changed, then a large payment | A supplier bank account, webhook address or payout destination is changed, then a high-value payment follows within 24 hours. Business-email fraud had 0% recovery in the GAB sample. | 30 points, review and hold | [Industry practice]. Param of SEC-09. Default: On. Basis: GS29 |
| GH-SEC-07 | Staff access, then customer takeover | Staff viewed or changed a customer's contact details or limits, then money leaves that customer's account within 24 hours. | 30 points, review (compliance only) | [Industry practice]. Param of INS-02. Default: On. Basis: GS28, GS29 (insiders) |
| GH-SEC-08 | CSA incident and vulnerability clocks | If the institution is a designated critical information infrastructure owner, report significant incidents to the Cyber Security Authority within 24 hours of detection and disclose vulnerabilities within 72 hours. | Legal clock: 24h incident, 72h vulnerability | [Regulatory]. Param of Nigeria section 8 incident clocks. Default: On. Basis: GS18, GS19 (secondary) |
| GH-SEC-09 | Report to Bank of Ghana within 10 days | A payment institution must report suspected or confirmed fraud, a security breach, a material service interruption or loss of confidential data to the Bank of Ghana within 10 days. A faster clock may exist in the 2026 cyber directive but was not found. | Legal clock: 10 days outer limit; faster placeholder | [Regulatory]. New local (Nigeria clock is 24h to CBN). Default: On. Basis: GS24 (s.42(5), s.42(3)); GS25 |
| GH-SEC-10 | Outsourcing notice and SLA filing | Before outsourcing a function, give the Bank of Ghana written notice; file the signed service agreement within 10 days of signing. | Task with deadline | [Regulatory]. New local. Default: On. Basis: GS24 (s.38) |
| GH-SEC-11 | Audit log completeness and 6-year retention | Every user action on the payment platform must be logged and kept for at least 6 years. A gap or an early deletion raises an alert. | Hard: deny delete; alert on gaps | [Regulatory]. Stricter than GOV-08 (6 years for logs). Default: On. Basis: GS24 (ss.20, 34) |
| GH-SEC-12 | Personal-data breach notice to the DPC | Reasonable grounds to believe personal data was accessed or acquired by an unauthorised person. Notify the Data Protection Commission and affected people as soon as reasonably practicable. The law has no hour count; 72 hours is a design default. | Task: notify DPC; internal clock 72h | [Regulatory]. Param of Nigeria NDPA 72h clock (Ghana statute has no hour count). Default: On. Basis: GS26 |
| GH-SEC-13 | Personal data leaving Ghana | Personal data is sent abroad without recorded consent and safeguards. A Bank of Ghana approval requirement under the 2026 directive was reported by a vendor and is unconfirmed, so that part ships off. | Flag; hold the transfer job | [Regulatory]. New local. Default: Off (threshold to be confirmed). Basis: GS26 (Templars); GS25 (Group-IB, vendor) |
| GH-SEC-14 | Cybersecurity vendor licence check | A penetration tester or security provider is used without a valid Cyber Security Authority licence. Unlicensed provision is unlawful and the CSA has enforced licensing from 31 January 2026. | Block vendor onboarding | [Regulatory]. New local. Default: On. Basis: GS20 |

#### Governance presets (GH-GOV)

| ID | Rule | What we look for (plain English) | Action or points | Tag, lineage, default, basis |
|---|---|---|---|---|
| GH-GOV-01 | Record keeping | Customer identity files, transaction records and reports are kept 5 years (after the relationship ends, from the transaction, or from the report date). Payment-platform audit logs are kept 6 years. | Deny deletion before the period ends | [Regulatory]. Param of GOV-08. Default: On. Basis: GS3 (s.32, per summary); GS24 |
| GH-GOV-02 | No tipping off | Nothing about an STR, a suspicion flag, an FIC order or a freeze appears in customer-facing screens, API responses, webhooks or exports. Failure to report carries 500-4,000 penalty units and up to 5 years in prison. | Strip fields and log | [Regulatory]. Param of GOV-09. Default: On. Basis: GS3 (s.38(4)) |
| GH-GOV-03 | Approved reporting to the FIC | STRs, CTRs and electronic transfer reports are prepared by the system but a compliance officer approves before submission. The goAML channel is claimed by a vendor but not confirmed, so the channel is a tenant setting. | Hold for approval | [Regulatory]. Param of GOV-06. Default: On. Basis: GS11 (goAML claim, unverified) |
| GH-GOV-04 | Customer review cycle | Existing customers are reviewed at least every two years, with enhanced ongoing monitoring for PEPs. | Create review tasks | [Regulatory]. New local. Default: On. Basis: GS2 (s.2.4.5) |
| GH-GOV-05 | Annual testing and twice-yearly training | The AML system is tested at least yearly and staff are trained at least twice a year. | Readiness check | [Regulatory]. New local. Default: On. Basis: GS2 (ss.2.10, 2.3.3) |
| GH-GOV-06 | Continuous screening and UN list drills | Sanctions, PEP and adverse-media checks at onboarding and continuously, with periodic drills and quarterly reporting where E.I. 2 requires them. | Run GH-REG-07 and GH-REG-08 for all parties | [Regulatory]. Param of GOV-05. Default: On. Basis: GS7 |
| GH-GOV-07 | Forex bureau compliance calendar | Forex bureaux file an annual audit by 30 April and a training submission by 31 December, and fix findings within 45 days. | Calendar tasks | [Regulatory]. New local. Default: On. Basis: GS9 |
| GH-GOV-08 | Data protection registration and impact assessments | Register with the Data Protection Commission before processing and renew every 2 years; run an impact assessment for high-risk processing. | Readiness check | [Regulatory]. New local. Default: On. Basis: GS26 |
| GH-GOV-09 | Critical-infrastructure governance | A designated critical information infrastructure owner appoints a cybersecurity officer, keeps a 24/7 contact for the CSA and undergoes regular compliance audits. | Readiness check | [Regulatory]. New local. Default: On. Basis: GS19 (secondary) |
| GH-GOV-10 | Digital credit provider licence check | A digital lender cannot be activated without its Bank of Ghana licence under the September 2025 digital credit directive. | Deny activation | [Regulatory]. Param of GOV-10. Default: On. Basis: GS32 |
| GH-GOV-11 | Virtual-asset provider licence status | A virtual-asset tenant records its Bank of Ghana or SEC registration, licence or waiver. Licensing is not yet live, so the check ships off and VASPs are treated as accountable institutions meanwhile. | Readiness check when enabled | [Regulatory]. New local. Default: Off (threshold to be confirmed). Basis: GS12, GS13, GS15 |
| GH-GOV-12 | Internal service levels | Triage a high-risk alert within 4 hours and reach an STR decision within 12 hours, inside the 24-hour statutory clock. | Escalate on breach | [My design]. Param of Nigeria recommended SLAs. Default: On. Basis: My design |
| GH-GOV-13 | Penalty exposure register | Each regulatory rule shows its penalty in penalty units and a cedi estimate, so the board sees the cost of a missed duty. One penalty unit is GHS 12 per a GRA page; the effective date is unverified. | Display only | [My design]. New local. Default: On. Basis: GS3, GS5, GS6, GS24 |

### 4.2 WHEN/IF/THEN logic for engineers

#### GH-REG: mandatory controls

```
RULE GH-REG-01 | control=hard | action=decline
WHEN  txn.type IN (wallet.send, wallet.cash_out, wallet.pay)
SCOPE tenant.type IN (emi, bank, psp) AND account.type = mobile_money_wallet
WINDOW 1d (calendar day, Africa/Accra) BY wallet.id
IF    SUM(txn.amount) + txn.amount > tenant.cfg.gh_tier_daily[wallet.kyc_tier]
      # default Minimum GHS 3_000 | Medium GHS 15_000 | Enhanced GHS 25_000 (BoG via Telecoms Chamber, 1 Mar 2024)
      # assumption: debits only count; direction scope to be confirmed with the operator guideline
THEN  decision=decline  reason=wallet_daily_limit

RULE GH-REG-02 | control=hard | action=decline_credit
WHEN  txn.type = wallet.credit
SCOPE tenant.type IN (emi, bank, psp) AND account.type = mobile_money_wallet
IF    wallet.balance + txn.amount > tenant.cfg.gh_tier_balance[wallet.kyc_tier]
      # default Minimum GHS 5_000 | Medium GHS 40_000 | Enhanced GHS 75_000
THEN  decision=decline_credit  reason=wallet_balance_cap  notify=sender_operator

RULE GH-REG-03 | control=hard | action=decline
WHEN  txn.type IN (wallet.send, wallet.cash_out, wallet.pay)
SCOPE tenant.type IN (emi, bank, psp) AND account.type = mobile_money_wallet
WINDOW 30d (calendar month, Africa/Accra) BY wallet.id
IF    tenant.cfg.gh_tier_monthly[wallet.kyc_tier] IS NOT NULL
AND   SUM(txn.amount) + txn.amount > tenant.cfg.gh_tier_monthly[wallet.kyc_tier]
      # default Minimum GHS 10_000 | Medium NULL | Enhanced NULL   # threshold to be confirmed; disabled-by-default for Medium and Enhanced
THEN  decision=decline  reason=wallet_monthly_limit

RULE GH-REG-04 | control=hard | action=report
WHEN  txn.type IN (cash.deposit, cash.withdrawal, cash.payment)
SCOPE txn.domestic = true
IF    txn.amount >= tenant.cfg.gh_ctr[tenant.type]
      # default bank, securities: GHS 50_000 | sdi, forex_bureau: GHS 20_000
      # >= used for safety; strict-or-equal wording not confirmed. Cash-only scope is an inference
THEN  task=report.CTR  due=tenant.cfg.gh_ctr_due          # default 24h after txn; FIC page says "filed daily"
      to=FIC  channel=tenant.cfg.fic_channel  gov=GH-GOV-03

RULE GH-REG-05 | control=hard | action=report
WHEN  txn.type IN (transfer.international.in, transfer.international.out)
SCOPE tenant.type IN (bank, sdi, emi) AND tenant.cfg.ectr_reporter = true   # FIC page names commercial banks as reporters
IF    txn.amount_usd_equiv >= tenant.cfg.gh_ectr                            # default USD 1_000
THEN  task=report.ECTR  due=24h_after_txn  to=FIC  gov=GH-GOV-03

RULE GH-REG-06 | control=hard | action=report
WHEN  alert.suspicion_recorded OR alert.suspicion_confirmed
IF    alert.suspicion_start_ts IS NOT NULL          # clock starts at the earliest recorded suspicion (s.38 wording), not at analyst sign-off (BoG wording)
THEN  task=report.STR  due=tenant.cfg.gh_str_due   # default 24h from suspicion_start_ts; no minimum amount
      to=FIC  gov=GH-GOV-02, GH-GOV-03

RULE GH-REG-07 | control=hard | action=edd
WHEN  customer.onboarded OR customer.periodic_review OR txn.created
IF    customer.is_pep = true OR ANY(beneficial_owner.is_pep = true)
THEN  decision=review  require=edd,source_of_funds,source_of_wealth,senior_approval  reason=pep
      add_to=pep_transaction_report  to=FIC  review_cycle=tenant.cfg.gh_pep_review      # default 2y

RULE GH-REG-08 | control=hard | action=block
WHEN  txn.created OR customer.onboarded OR list.updated
IF    screen(party.name, party.id, party.account) MATCHES lists(un_consolidated, gh_designations, tenant_lists)
        USING fuzzy + transliteration  score >= tenant.cfg.match_threshold
AND   match.reviewed = confirmed OR tenant.cfg.auto_block_on_strong_match = true
THEN  decision=block  freeze=account  case=open  task=report.FIC  reason=sanctions_match
      freeze_sla=tenant.cfg.gh_freeze_sla     # PLACEHOLDER 24h [My design]; E.I. 2 says freeze without prior notice, no hour count retrieved
      gov=GH-GOV-02

RULE GH-REG-09 | control=hard | action=block
WHEN  account.open OR customer.onboarded OR kyc.refresh
SCOPE tenant.type = forex_bureau OR tenant.cfg.require_ghana_card = true
IF    identity.ghana_card_verified != true            # NIA verification result stored by the identity provider interface
THEN  decision=block  reason=ghana_card_not_verified  require=collect_ghana_card

RULE GH-REG-10 | control=hard | action=block
WHEN  account.open OR kyc.refresh OR daily_job
IF    ANY OF ( kyc.identity_verified = false , customer.name IS NULL , account.nominal_holder = true AND beneficial_owner IS NULL )
THEN  decision=block  restrict=account  case=open  reason=anonymous_or_fictitious_account

RULE GH-REG-11 | control=hard | action=hold
WHEN  order.received
SCOPE order.source = FIC AND order.type IN (suspension, freeze)
THEN  hold=transaction_or_account  hold_max=7_working_days  expire=auto  audit=log
      visibility=compliance_only  gov=GH-GOV-02
      # s.45 suspension up to 7 working days; s.56 freeze, court application within 7 working days (E.I. 2 summary says 48h: unresolved)

RULE GH-REG-12 | control=hard | action=edd
WHEN  txn.type IN (forex.buy, forex.sell)
SCOPE tenant.type = forex_bureau
IF    txn.amount_usd_equiv >= tenant.cfg.gh_forex_edd        # default USD 10_000
THEN  decision=review  require=edd,biometric_verification,ghana_card  reason=forex_high_value

RULE GH-REG-13 | control=hard | action=hold
WHEN  crypto.transfer
SCOPE tenant.type = vasp AND tenant.cfg.gh_travel_rule_enabled = true
IF    txn.amount_usd_equiv >= tenant.cfg.gh_travel_rule_min      # PLACEHOLDER NULL: threshold to be confirmed; disabled-by-default
AND   ANY OF ( originator.data_complete = false , beneficiary.data_complete = false )
THEN  decision=hold  reason=travel_rule_data_missing  link=CRY-13
```

#### GH-TM: typology and monitoring

```
RULE GH-TM-01 | weight=30 | action=review
WHEN  txn.type IN (transfer.outbound, wallet.send)
WINDOW 30m BY account.id
IF    EXISTS(device.bound WHERE device.is_new = true)
AND   EXISTS(beneficiary.added WHERE beneficiary.is_new = true)
AND   txn.amount > tenant.cfg.floor                     # suggested default GHS 2_000 (average MoMo ticket about GHS 528, Dec 2025)
THEN  signal=GH-TM-01  evidence=(device.id, beneficiary.id, times)

RULE GH-TM-02 | weight=25 | action=step_up
WHEN  txn.type IN (transfer.outbound, wallet.send, wallet.cash_out)
IF    ANY OF ( telco.sim_swap_flag = true , EXISTS(sim.reregistered OVER tenant.cfg.sim_swap_window) , EXISTS(device.reregistered OVER tenant.cfg.sim_swap_window) )
      # default window 72h
AND   txn.amount > tenant.cfg.floor
THEN  signal=GH-TM-02  evidence=(swap_source, swap_time)

RULE GH-TM-03 | weight=25 | action=step_up
WHEN  txn.type = transfer.outbound
IF    ANY OF ( EXISTS(profile.password_changed OVER 24h) , EXISTS(profile.pin_changed OVER 24h) ,
               EXISTS(profile.phone_changed OVER 24h) , EXISTS(profile.email_changed OVER 24h) )
AND   txn.amount > tenant.cfg.floor
THEN  signal=GH-TM-03  evidence=(change_type, change_time)

RULE GH-TM-04 | weight=10 or 20
WHEN  session.login OR txn.created
IF    geo.country NOT IN profile.usual_countries THEN signal=GH-TM-04 weight=10
IF    distance(geo, previous.geo) / time_between > 900 km/h THEN signal=GH-TM-04 weight=20   # impossible travel

RULE GH-TM-05 | weight=20
WHEN  txn.type IN (transfer.outbound, wallet.send)
WINDOW 24h BY account.id
IF    SUM(txn.amount) > 3 * AVG(daily_outbound OVER 30d)
AND   SUM(txn.amount) > tenant.cfg.floor                 # suggested GHS 5_000 for retail wallets; tune per segment
THEN  signal=GH-TM-05  evidence=(sum_24h, avg_30d)

RULE GH-TM-06 | weight=15
WHEN  txn.type IN (transfer.outbound, wallet.send)
WINDOW 10m BY account.id
IF    COUNT(txn) >= 5
THEN  signal=GH-TM-06  evidence=(count, window)

RULE GH-TM-07 | weight=25 | action=review
WHEN  txn.type IN (wallet.send, wallet.cash_out)
SCOPE account.type = mobile_money_wallet
IF    ANY OF ( COUNT(txn WHERE txn.amount BETWEEN 0.90*L AND 0.999*L OVER 24h BY wallet.id) >= 3 ,
               COUNT(DISTINCT day WHERE SUM(txn.amount)/L >= 0.80 OVER 7d BY wallet.id) >= 3 )
      WHERE L = tenant.cfg.gh_tier_daily[wallet.kyc_tier]
THEN  signal=GH-TM-07  evidence=(limit, amounts)

RULE GH-TM-08 | weight=25 | action=review
WHEN  txn.type IN (cash.deposit, cash.withdrawal)
WINDOW 5d BY customer.id INCLUDING linked_accounts, branches
IF    COUNT(txn WHERE txn.amount BETWEEN 0.90*T AND 0.999*T) >= 3
      WHERE T = tenant.cfg.gh_ctr[tenant.type]            # GHS 50_000 or GHS 20_000; FIC publishes no separate band
THEN  signal=GH-TM-08  evidence=(threshold, amounts, branches)

RULE GH-TM-09 | weight=20
WHEN  txn.type = transfer.inbound
WINDOW 24h BY account.id
IF    COUNT(DISTINCT sender.id) >= 10
THEN  signal=GH-TM-09

RULE GH-TM-10 | weight=25 | action=review
WHEN  txn.type IN (transfer.outbound, wallet.send)
WINDOW 60m BY account.id
IF    SUM(txn.amount WHERE direction=out) >= 0.80 * SUM(txn.amount WHERE direction=in)
AND   SUM(txn.amount WHERE direction=in) > tenant.cfg.floor
THEN  signal=GH-TM-10  evidence=(in_sum, out_sum, ratio)

RULE GH-TM-11 | weight=20
WHEN  txn.type IN (transfer.outbound, wallet.send)
WINDOW 60m BY source_inflow.id
IF    COUNT(DISTINCT beneficiary.id) >= 5
THEN  signal=GH-TM-11

RULE GH-TM-12 | weight=15
WHEN  txn.type IN (atm.withdrawal, pos.purchase, wallet.cash_out)
WINDOW 30m BY account.id
IF    EXISTS(inflow WHERE source IN (card, p2p, wallet))
AND   txn.amount >= 0.70 * inflow.amount
THEN  signal=GH-TM-12

RULE GH-TM-13 | weight=20
WHEN  txn.created
SCOPE account.age < 7d
IF    SUM(txn.amount OVER account.age) > 10 * kyc_tier.norm(account.kyc_tier)
THEN  signal=GH-TM-13  evidence=(volume, tier_norm)

RULE GH-TM-14 | weight=20
WHEN  txn.created
IF    account.days_since_last_activity >= 90
AND   txn.amount > tenant.cfg.large_after_dormancy        # suggested GHS 5_000
THEN  signal=GH-TM-14

RULE GH-TM-15 | weight=25 | action=review
WHEN  account.created OR application.submitted OR txn.created
IF    ANY OF ( COUNT(DISTINCT account.id BY device.id) >= 3 ,
               COUNT(DISTINCT account.id BY ghana_card_no) >= 2 ,
               COUNT(DISTINCT account.id BY phone) >= 2 ,
               COUNT(DISTINCT account.id BY address) >= 2 )
THEN  signal=GH-TM-15  evidence=(shared_identifier, linked_accounts)

RULE GH-TM-16 | weight=25 | action=review
WHEN  agent.daily_job
SCOPE agent.active = true
IF    ANY OF ( agent.float_topup_24h > tenant.cfg.float_peer_mult * peer_group.p95(float_topup_24h) ,
               (agent.cash_in_24h + agent.cash_out_24h) / agent.float_balance > tenant.cfg.float_cycle_max ,
               agent.share_new_wallets_7d > tenant.cfg.new_wallet_share_max )
      # PLACEHOLDER values: threshold to be confirmed; disabled-by-default; calibrate on operator data
THEN  signal=GH-TM-16  evidence=(agent.id, float_metrics)  visibility=compliance_only

RULE GH-TM-17 | weight=30 | action=review
WHEN  eod.reconciliation OR staff.txn
SCOPE staff.role IN (teller, relationship_officer, direct_sales_agent)
IF    ANY OF ( ABS(cash_received_recorded - cash_posted_to_ledger) > tenant.cfg.variance_tolerance ,   # default GHS 0
               COUNT(staff.initiated_debit WHERE account.dormant = true OVER 30d) >= 1 ,
               staff.manual_override_count OVER 30d > peer_group.p95(manual_override_count) )
THEN  signal=GH-TM-17  visibility=compliance_only  evidence=(staff.id, variance)   # gov=GH-GOV-02

RULE GH-TM-18 | weight=20
WHEN  daily_job
SCOPE account.type = personal
WINDOW 30d BY account.id
IF    SUM(txn.amount WHERE direction=in) > 5 * peer_group.p95(SUM OVER 30d)
AND   ANY OF ( COUNT(txn WHERE counterparty.type = business) >= tenant.cfg.pb_transfers_min ,   # default 5
               COUNT(txn WHERE memo MATCHES (loan, borrow) AND counterparty.is_shareholder = true) >= 1 ,
               linked_relatives_accounts.volume > tenant.cfg.relative_volume_floor )
THEN  signal=GH-TM-18  evidence=(turnover, counterparties)

RULE GH-TM-19 | weight=25 | action=review
WHEN  txn.type = transfer.inbound OR cash.withdrawal
WINDOW 7d BY account.id
SCOPE account.type IN (personal, sme)
IF    SUM(txn.amount WHERE counterparty.tag IN (gold_buyer, gold_aggregator, forex_bureau)) >= tenant.cfg.gold_inflow_floor
      # PLACEHOLDER NULL: threshold to be confirmed; disabled-by-default
AND   SUM(cash_out OVER 48h after inflow) >= 0.70 * inflow
THEN  signal=GH-TM-19  evidence=(counterparties, amounts)

RULE GH-TM-20 | weight=30 | action=review
WHEN  txn.created
IF    counterparty.name_or_id IN list.bog_unlicensed_digital_lenders
OR    counterparty.tag = unlicensed_lender
THEN  signal=GH-TM-20  evidence=(counterparty, list_version)

RULE GH-TM-21 | weight=20
WHEN  txn.type IN (transfer.outbound, wallet.send)
WINDOW 30m BY account.id
IF    EXISTS(loan.disbursement)
AND   SUM(txn.amount) >= 0.90 * loan.disbursement.amount
THEN  signal=GH-TM-21

RULE GH-TM-22 | weight=25 | action=review
WHEN  txn.type = transfer.inbound
WINDOW 7d BY account.id
IF    COUNT(DISTINCT sender.id WHERE unrelated = true) >= 20
AND   SUM(outflow WHERE recipient IN prior_senders) >= 0.30 * SUM(outflow)
THEN  signal=GH-TM-22  evidence=(senders, recycled_share, memo_flags)

RULE GH-TM-23 | weight=25 | action=review
WHEN  txn.type IN (transfer.outbound, wallet.send)
WINDOW 2h BY account.id
IF    EXISTS(inflow WHERE sender.is_first_time = true AND inflow.amount >= tenant.cfg.wrong_transfer_min)   # PLACEHOLDER, disabled-by-default
AND   txn.beneficiary != inflow.sender
AND   txn.amount >= 0.80 * inflow.amount
THEN  signal=GH-TM-23

RULE GH-TM-24 | weight=20
WHEN  txn.type = transfer.interoperable.out
WINDOW 60m BY wallet.id
IF    wallet.age < 7d
AND   SUM(txn.amount WHERE interoperable = true) >= 0.80 * SUM(txn.amount WHERE direction=in)
AND   SUM(txn.amount WHERE direction=in) > tenant.cfg.floor
      # threshold to be confirmed; disabled-by-default
THEN  signal=GH-TM-24

RULE GH-TM-25 | weight=25 | action=review
WHEN  txn.type = transfer.outbound
WINDOW 72h BY account.id
IF    EXISTS(inflow WHERE inflow.type = transfer.international.in AND inflow.amount_usd_equiv >= tenant.cfg.gh_ectr)
AND   COUNT(DISTINCT beneficiary.id) >= 3
AND   SUM(txn.amount) >= 0.70 * inflow.amount
THEN  signal=GH-TM-25  evidence=(inflow.sender, beneficiaries)   # optional boost +5 if sender.age >= 65

RULE GH-TM-26 | weight=25 | action=review
WHEN  txn.type IN (forex.buy, forex.sell)
SCOPE tenant.type = forex_bureau
WINDOW 7d BY customer.id
IF    COUNT(txn WHERE txn.amount_usd_equiv BETWEEN 0.90*tenant.cfg.gh_forex_edd AND 0.999*tenant.cfg.gh_forex_edd) >= 2
THEN  signal=GH-TM-26

RULE GH-TM-27 | weight=10
WHEN  txn.type IN (transfer.international.in, transfer.international.out)
IF    counterparty.country IN tenant.cfg.fatf_increased_monitoring_list     # list maintained by tenant; 23 jurisdictions at the time of the advisory
THEN  signal=GH-TM-27  require=risk_based_scrutiny   # not blanket EDD

RULE GH-TM-28 | weight=25 | action=review
WHEN  txn.type IN (transfer.outbound, wallet.send)
WINDOW 7d BY account.id
IF    COUNT(DISTINCT sender.id) >= 10
AND   SUM(txn.amount WHERE counterparty.tag IN (vasp, p2p_crypto, crypto_linked)) >= 0.70 * SUM(txn.amount WHERE direction=in)
THEN  signal=GH-TM-28  link=CRY-01,CRY-08
```

#### GH-SEC: cybersecurity

```
RULE GH-SEC-01 | weight=30
WHEN  auth.failure
WINDOW 5m BY ip
IF    COUNT(auth.failure) >= 10 AND COUNT(DISTINCT account.id) >= 5
THEN  signal=GH-SEC-01  action_hint=rate_limit_ip

RULE GH-SEC-02 | weight=25
WHEN  mfa.prompt_sent OR otp.used
WINDOW 10m BY account.id
IF    COUNT(mfa.prompt_sent WHERE type IN (push, otp)) >= 5
   OR EXISTS(otp.used WHERE otp.requesting_device != otp.using_device)
THEN  signal=GH-SEC-02  action_hint=pause_prompts

RULE GH-SEC-03 | weight=30
WHEN  api.request
SCOPE auth.type = api_key
IF    ANY OF ( request.network NOT IN key.known_networks , request.country NOT IN key.known_countries ,
               RATE(request OVER 5m) > 5 * key.baseline_rate ,
               RATIO(status IN (400, 404) OVER 10m) > tenant.cfg.probe_ratio )
THEN  signal=GH-SEC-03  action_hint=throttle_key

RULE GH-SEC-04 | weight=30
WHEN  data.export
WINDOW 1h BY user.id
IF    EXISTS(user.role_changed)
THEN  signal=GH-SEC-04

RULE GH-SEC-05 | weight=25
WHEN  data.export
IF    export.rows > 5 * BASELINE(export.rows BY user.id OVER 30d)
   OR export.rows > 5 * BASELINE(export.rows BY tenant.id OVER 30d)
THEN  signal=GH-SEC-05  link=GH-SEC-12

RULE GH-SEC-06 | weight=30 | action=review
WHEN  txn.created
WINDOW 24h BY customer.id OR merchant.id
IF    EXISTS(config.changed WHERE field IN (webhook_url, payout_destination, bank_account, supplier_account))
AND   txn.amount > tenant.cfg.high_value             # suggested GHS 5_000 retail; GHS 50_000 for bank e-transfer tier
THEN  signal=GH-SEC-06  action_hint=hold_payout

RULE GH-SEC-07 | weight=30 | action=review
WHEN  txn.type = transfer.outbound
WINDOW 24h BY customer.id
IF    EXISTS(staff_action WHERE type IN (view_contact, change_contact, change_limits))
THEN  signal=GH-SEC-07  evidence=(staff.id, action, time)  visibility=compliance_only

RULE GH-SEC-08 | control=clock | action=report
WHEN  incident.detected
SCOPE tenant.cii_designated = true
IF    incident.severity IN tenant.cfg.csa_significant_levels
THEN  task=notify.CSA  due=24h_from_detection  contact=24x7  gov=GH-GOV-09
WHEN  vulnerability.confirmed
SCOPE tenant.cii_designated = true
THEN  task=disclose.CSA  due=72h
      # whether the clocks cover all financial institutions or only designated CII is unconfirmed (primary text of Act 1038 not retrieved)

RULE GH-SEC-09 | control=clock | action=report
WHEN  incident.detected
SCOPE tenant.type IN (emi, psp, payment_institution)
IF    incident.type IN (fraud_suspected, fraud_confirmed, security_breach, material_service_interruption, confidential_data_loss)
THEN  task=report.BoG  due=MIN(10d, tenant.cfg.bog_cisd_clock)
      # tenant.cfg.bog_cisd_clock default NULL: BoG CISD 2026 hour clock not found; threshold to be confirmed
      # exposure: s.42(3) 500 penalty units plus 50 per day (about GHS 6,000 plus GHS 600 per day at GHS 12 per unit; inference)

RULE GH-SEC-10 | control=clock | action=task
WHEN  outsourcing.planned
SCOPE tenant.type IN (emi, psp, payment_institution)
THEN  task=notify.BoG  due=before_signing
WHEN  outsourcing.signed
THEN  task=submit.SLA_to_BoG  due=10d_after_signing

RULE GH-SEC-11 | control=hard | action=deny
WHEN  audit_log.integrity_job OR record.delete_requested
IF    audit_log.coverage < 100%  THEN alert=log_gap
IF    record.type = audit_log AND record.age < 6y THEN deny_delete reason=act987_log_retention

RULE GH-SEC-12 | control=clock | action=report
WHEN  breach.confirmed
IF    breach.personal_data = true AND breach.unauthorised_access = true
THEN  task=notify.DPC  due=tenant.cfg.dpc_internal_clock       # default 72h [My design]; statute: as soon as reasonably practicable
      task=notify.data_subjects  content=(nature, measures, likely_consequences)

RULE GH-SEC-13 | control=hard | action=flag
WHEN  data.transfer
SCOPE destination.country != GH AND data.class = personal
IF    NOT ( consent.recorded = true AND safeguards.recorded = true )
THEN  flag=cross_border_transfer  hold=transfer_job
IF    tenant.cfg.cisd_bog_transfer_approval = true AND transfer.bog_approval IS NULL     # default false: threshold to be confirmed; disabled-by-default
THEN  hold=transfer_job  reason=bog_approval_missing

RULE GH-SEC-14 | control=hard | action=block
WHEN  vendor.onboard OR vendor.annual_review
SCOPE vendor.service = cybersecurity
IF    vendor.csa_licence_status != valid
THEN  decision=block  reason=unlicensed_cybersecurity_provider
```

#### GH-GOV: governance

```
RULE GH-GOV-01 | type=control
WHEN  record.delete_requested
IF    ( record.type IN (customer_file) AND record.age < tenant.cfg.gh_retention_years after relationship_end )
   OR ( record.type IN (transaction, report) AND record.age < tenant.cfg.gh_retention_years after (txn OR report_date) )
      # default 5y; audit logs 6y under GH-SEC-11
THEN  deny_delete  reason=retention

RULE GH-GOV-02 | type=control
WHEN  api.response OR webhook.send OR export.generate
IF    payload CONTAINS (str.exists OR str.status OR fic_order OR flag.visibility = compliance_only)
AND   recipient.role NOT IN (compliance_officer, mlro)
THEN  strip_fields  AND  log=audit

RULE GH-GOV-03 | type=workflow
WHEN  report.draft_ready (STR, CTR, ECTR)
IF    report.approved_by IN roles(compliance_officer)
THEN  allow_submit  via=tenant.cfg.fic_channel     # goAML not confirmed for Ghana
ELSE  hold_for_approval

RULE GH-GOV-04 | type=job | schedule=daily
IF    customer.last_review_age >= tenant.cfg.gh_review_cycle      # default 2y
THEN  task=periodic_review

RULE GH-GOV-05 | type=job | schedule=daily
IF    aml_system.last_tested > 12mo OR staff.training_events_last_12mo < 2
THEN  readiness=fail  reason=bog_guideline_testing_training

RULE GH-GOV-06 | type=control
WHEN  customer.onboarded OR list.updated OR daily_job
THEN  run GH-REG-08 AND GH-REG-07 AND adverse_media_screen FOR customer, beneficial_owner, counterparty
WHEN  quarter.end
THEN  task=sanctions_report_and_drill   # E.I. 2 per summary; verify recipient and format

RULE GH-GOV-07 | type=calendar
SCOPE tenant.type = forex_bureau
WHEN  date = 30-Apr  THEN task=submit.annual_audit
WHEN  date = 31-Dec  THEN task=submit.training_report
WHEN  audit.finding_opened  THEN task=remediate  due=45d

RULE GH-GOV-08 | type=job | schedule=daily
IF    tenant.dpc_registration.expiry < now + 90d OR tenant.dpc_registration IS NULL
THEN  readiness=fail  reason=dpc_registration
IF    processing.high_risk = true AND dpia.completed = false
THEN  readiness=fail  reason=dpia_missing

RULE GH-GOV-09 | type=policy
SCOPE tenant.cii_designated = true
IF    tenant.cyber_officer IS NULL OR tenant.csa_24x7_contact IS NULL OR cii_audit.last_done > tenant.cfg.cii_audit_cycle
THEN  readiness=fail  reason=cii_governance

RULE GH-GOV-10 | type=policy
WHEN  tenant.activate
IF    tenant.type = digital_lender AND tenant.bog_dcsp_licence IS NULL
THEN  deny_activate  reason=missing_bog_digital_credit_licence

RULE GH-GOV-11 | type=policy
WHEN  tenant.activate
SCOPE tenant.type = vasp AND tenant.cfg.gh_vasp_check_enabled = true
IF    tenant.vasp_route NOT IN (registration, licence, waiver)
THEN  deny_activate  reason=missing_act1154_status      # disabled-by-default: operational licensing expected by 2027
ELSE  treat_as=accountable_institution  apply=GH-REG-04, GH-REG-05, GH-REG-06

RULE GH-GOV-12 | type=sla
WHEN  alert.created
IF    alert.risk = high AND NOW - alert.created > 4h AND alert.triaged = false   THEN escalate
IF    alert.suspicion_recorded AND NOW - alert.suspicion_start_ts > 12h AND alert.str_decision IS NULL   THEN escalate

RULE GH-GOV-13 | type=display
THEN  show(rule.penalty_units, rule.penalty_units * tenant.cfg.gh_penalty_unit_value)    # default GHS 12 (inference: GRA page citing Act 572)
```

## 5. Rwanda preset rules

Notation and conventions are the same as section 4; money is whole RWF and windows use Africa/Kigali. Source keys (RS1 and so on) are listed in section 7. Where a number was not found, the rule says so and ships off.

### 5.1 Plain-English tables

#### Mandatory controls (RW-REG)

| ID | Rule | What we look for (plain English) | Action or points | Tag, lineage, default, basis |
|---|---|---|---|---|
| RW-REG-01 | Cross-border cash declaration above US$10,000 | Cash or bearer instruments above US$10,000 (or equivalent) brought into or out of Rwanda must be declared. A deposit of foreign cash above that amount with no declaration reference is flagged. | Hard: flag, open case, report to the FIC | [Regulatory]. Analogue of REG-10 (physical cash, not wires). Default: On. Basis: RS5 (Reg 002/FIC/2021 Art 4, Art 9); verify still in force after Law 001/2025 |
| RW-REG-02 | Freeze within 24 hours | A person or entity subject to targeted financial sanctions has funds in the institution. 'Immediately' means within 24 hours at the latest, so the freeze clock starts when the designation is received. | Hard: freeze and report; 24-hour deadline | [Regulatory]. Param of REG-11 (Rwanda defines the freeze clock). Default: On. Basis: RS3 (Art 2(b), Art 4); aggregator text |
| RW-REG-03 | Sanctions and TFS screening | A name, ID or account matches UN or Rwandan targeted-sanctions lists, including spelling variants. Missing the duty carries a fine of RWF 1,000,000 to RWF 10,000,000. | Hard: block and open a freeze case | [Regulatory]. Param of REG-11. Default: On. Basis: RS2 (FIC Reg 001/FIC/2025 Arts 26-27) |
| RW-REG-04 | Suspicious transaction report (STR) | A person forms a suspicion that funds are linked to crime. The report goes to the FIC, without telling the customer. The numeric deadline in Law 001/2025 Art 32 was not found, so the clock is a tenant setting with a design default. | Report to the FIC; deadline is a placeholder | [Regulatory]. Param of REG-14 (deadline placeholder). Default: On. Basis: RS3 (Art 32, Art 34 per map); RS6 |
| RW-REG-05 | Cash and wire transaction report | Law 028/2023 Art 31 required reports of cash transactions and wire transfers; no amount was found in Law 001/2025 or FIC regulations. The rule ships off with a placeholder. | Report when enabled | [Regulatory]. Analogue of REG-17 (amount not found). Default: Off (threshold to be confirmed). Basis: RS4 (predecessor Art 31); RS3 |
| RW-REG-06 | Politically exposed person (PEP) | The customer or an owner behind a company is a PEP. Senior management approves, source of wealth is verified, and monitoring is enhanced. | Hard: enhanced due diligence and senior approval | [Regulatory]. Param of REG-12. Default: On. Basis: RS2 (R.12 now Compliant); RS3 (Art 22 per map) |
| RW-REG-07 | eKash transaction cap and fee ceiling | A person-to-person transfer on the national eKash rail is above RWF 10 million, or the fee charged is above RWF 20. Institutions may set lower limits. | Hard: decline the transfer; block the over-limit fee | [Regulatory]. New local. Default: On. Basis: RS12 (Directive 45/2026, secondary news) |
| RW-REG-08 | Daily cash withdrawal ceiling (RWF 5 million) | A customer withdraws more than RWF 5 million in cash in a day through any payment instrument. Banks should pay by crossed cheque or credit transfer instead, and report exceptions to the central bank weekly. The 2009 directive may no longer apply. | Hard when enabled: decline cash; log exceptions | [Regulatory]. Analogue of REG-07 and REG-09. Default: Off (verify first). Basis: RS9 (Directive 02/2009; verify currency) |
| RW-REG-09 | Maximum three e-money accounts per customer | A customer opens a fourth e-money account without central bank authorisation. Whether the cap is per issuer or across issuers is not confirmed. | Hard: decline the fourth account | [Regulatory]. New local. Default: On. Basis: RS11 (Reg 54/2022 Art 9) |
| RW-REG-10 | National ID verification, no anonymous accounts | An e-money account or wallet is opened without checking the national ID against the national database. | Hard: block opening | [Regulatory]. Analogue of REG-06 (NIDA instead of BVN or NIN). Default: On. Basis: RS11 (Reg 54/2022 Art 9) |
| RW-REG-11 | Authentication before e-money transactions | An e-money transaction is accepted without a PIN, password or other authentication step. | Hard: decline | [Regulatory]. Analogue of REG-03. Default: On. Basis: RS11 (Reg 54/2022 Art 10); BNR PDF |
| RW-REG-12 | Beneficial owner identified for companies | A company customer is opened or reviewed without its beneficial owners being identified and verified. The ownership percentage that counts is not found, so the tenant sets it. | Hard: block or review | [Regulatory]. New local (Nigeria has KYB under GOV-01). Default: On. Basis: RS4 (Art 33 predecessor); RS32 (FIC Guidelines 001/2023) |
| RW-REG-13 | Virtual-asset licensing gate | Virtual-asset business must go through a provider licensed by the Capital Market Authority. Virtual assets are not legal tender, and direct use as payment is barred unless the central bank allows it. | Hold and review | [Regulatory]. New local (cross-reference CRY rules). Default: On. Basis: RS13 (Law 023/2026) |
| RW-REG-14 | Virtual-asset Travel Rule data | A virtual-asset transfer above the Travel Rule limit lacks sender or receiver details. Law 023/2026 refers to the rule, but no threshold was found, so the rule ships off. | Hard when enabled: hold | [Industry practice]. Cross-reference CRY-13 (threshold not found). Default: Off (threshold to be confirmed). Basis: RS13 |

#### Typology and monitoring rules (RW-TM)

| ID | Rule | What we look for (plain English) | Action or points | Tag, lineage, default, basis |
|---|---|---|---|---|
| RW-TM-01 | New account, big volume | An account or wallet under 7 days old moves far more than normal. Survey data put 7.7% of fraud at account creation versus 1.6% at login and 0.5% at transactions. | 20 points | [Industry practice]. Param of MUL-04 (limits are issuer-defined, so tier norm is a tenant value). Default: On. Basis: RS23 |
| RW-TM-02 | SIM replacement, then transaction | A SIM replacement or re-registration, then a login, one-time password or transaction within a configurable number of hours. No legal cooling-off period was found. | 25 points, step-up | [Industry practice]. Param of ATO-03 (window is a tenant value). Default: On. Basis: RS21, RS23 |
| RW-TM-03 | Repeated SIM changes on one national ID | The same national ID shows two or more SIM registrations or replacements in 30 days. A draft RURA rule caps SIMs at three per operator for Rwandan ID holders; the cap is unconfirmed. | 15 points | [Industry practice]. Param of SIM-02 (draft cap on SIMs per ID). Default: On. Basis: RS21 (draft) |
| RW-TM-04 | SIM not verified or foreign-linked, large load | A large value is loaded onto a SIM or wallet whose SIM is not verified against the national ID, or is linked to a foreign number. The March 2026 Equity Bank Rwanda case used such SIMs. | 20 points, review | [Industry practice]. Param of SIM-01 (national ID instead of NIN). Default: On. Basis: RS24, RS21 |
| RW-TM-05 | New device, new payee, then transfer | Within 30 minutes: a new device is linked, a new payee is added, and money is sent above the minimum. | 30 points, review | [Industry practice]. Param of ATO-01. Default: On. Basis: RS23 (account takeover 24% of losses) |
| RW-TM-06 | Credentials changed, then transfer | Password, PIN, phone or email changed in the last 24 hours, then money is sent above the minimum. | 25 points, step-up | [Industry practice]. Param of ATO-02. Default: On. Basis: RS23 |
| RW-TM-07 | Money passing straight through (mule) | 80% or more of money received leaves within 60 minutes. Mule scams were the largest loss category (29%) in the survey sample. | 25 points, review | [Industry practice]. Param of MUL-01. Default: On. Basis: RS23, RS25 |
| RW-TM-08 | Many senders into one account | Ten or more different senders pay one account in 24 hours. | 20 points | [Industry practice]. Param of MUL-02. Default: On. Basis: RS23 |
| RW-TM-09 | One payment split many ways | One incoming payment is split among five or more recipients within 60 minutes. | 20 points | [Industry practice]. Param of MUL-03. Default: On. Basis: RS23 |
| RW-TM-10 | Dormant account wakes up | No activity for 90 days or more, then a large payment in or out. The 2024 risk assessment lists dormant-account reactivation as an indicator. | 20 points | [Industry practice]. Param of MUL-05. Default: On. Basis: RS25 (NRA indicator) |
| RW-TM-11 | Shared national ID, phone or device across accounts | One device is used on three or more accounts, or one national ID, phone number or address appears on several accounts or applications. | 25 points, review | [Industry practice]. Param of MUL-06 (national ID added). Default: On. Basis: RS23, RS24 |
| RW-TM-12 | Splitting to stay under ceilings | Three or more transactions in 24 hours each between 90% and 99.9% of a known limit: the RWF 5 million cash ceiling, the RWF 10 million eKash cap, or the institution's own limit. The 2024 risk assessment lists structured withdrawals. | 25 points, review | [Industry practice]. Param of VEL-04. Default: On. Basis: RS25, RS12, RS9 |
| RW-TM-13 | Large first payment to a new payee | A first large payment to a new payee, or cumulative payments of RWF 1 million to new payees in 24 hours. The median survey loss was RWF 869,249; the band is analyst judgement. | 20 points | [My design]. New local. Default: On. Basis: RS23 (median loss); band is my design |
| RW-TM-14 | Sudden spike against the customer's own pattern | Money sent in 24 hours is more than three times the usual daily amount, or a single amount is far above the customer's history. | 20 to 24 points | [Industry practice]. Param of VEL-02 and BEH-01. Default: On. Basis: Nigeria library |
| RW-TM-15 | Impossible travel or foreign location | A country the customer does not normally use (10 points), or two events too far apart to travel between in the time (20 points). | 10 or 20 points | [Industry practice]. Param of BEH-02. Default: On. Basis: Nigeria library |
| RW-TM-16 | Agent cash-out clustering and float manipulation | Several customers cash out at one agent in a short span, or an agent's float moves in ways its customers do not explain. The Equity Bank Rwanda case (about USD 3.4m) exploited float mechanisms. All numbers are placeholders. | 25 points, review | [My design]. New local. Default: Off (threshold to be confirmed). Basis: RS24 (single case); no float threshold found |
| RW-TM-17 | Remittance in, then many small cash-outs | An international remittance arrives and is withdrawn in many small cash-outs at several agents. A national risk assessment case paid agents RWF 20,000 per withdrawal. | 25 points, review | [Industry practice]. New local. Default: On. Basis: RS25 (NRA case) |
| RW-TM-18 | Real-estate payments | A large payment to a property seller or developer is funded by cash, many small deposits or a recently opened account whose income does not explain it. Real estate is rated Medium-High, the highest sector, in the 2024 risk assessment. | 25 points, review | [Industry practice]. New local. Default: Off (threshold to be confirmed). Basis: RS25 (NRA); thresholds not found |
| RW-TM-19 | Shell company pattern | A company with no visible business activity, a shared address or director, thin ownership detail and fast pass-through of funds. A crypto scam case moved USD 3.4m through 48 shell companies. | 30 points, review | [Industry practice]. New local. Default: On. Basis: RS25 (NRA, KT Press) |
| RW-TM-20 | Insider embezzlement | Staff-linked accounts move far more than peers, manual overrides run high, or staff touch a customer's details just before money leaves. Embezzlement is 39% of proceeds in the 2024 risk assessment. | 20 to 30 points, review (compliance only) | [Industry practice]. Param of INS-01 and INS-02. Default: On. Basis: RS25 |
| RW-TM-21 | Pyramid and Ponzi-style pooling | Twenty or more unrelated senders pay one account in 7 days and 30% or more of payouts go to earlier payers. The central bank has warned about pyramid schemes and scam coins since 2019. | 25 points, review | [Industry practice]. Param of PON-01. Default: On. Basis: RS29 |
| RW-TM-22 | Crypto exposure under Law 023/2026 | Many small credits followed by payments to exchanges or unlicensed virtual-asset counterparties. Cross-reference CRY-01, CRY-03 and CRY-08 for the on-chain rules. | 25 points, review | [My design]. New local (cross-reference CRY-01, CRY-03, CRY-08). Default: On. Basis: RS13, RS25 |
| RW-TM-23 | Foreign cash split to stay under the declaration line | Two or more foreign-cash deposits in 7 days, each between 90% and 99.9% of US$10,000. | 25 points, review | [My design]. New local (derived from the declaration limit). Default: On. Basis: RS5; derivation is my design |
| RW-TM-24 | SIM-box and bulk SIM pattern (telecom tenants) | A device or account uses 30 or more SIMs, or shows international call patterns that bypass legitimate routes. A 2017 SIM-box case cost RWF 1 billion. | 20 points, review | [Industry practice]. New local (telecom tenants only). Default: On. Basis: RS28 |

#### Cybersecurity rules (RW-SEC)

| ID | Rule | What we look for (plain English) | Action or points | Tag, lineage, default, basis |
|---|---|---|---|---|
| RW-SEC-01 | Penetration-test and vulnerability cadence | Run an annual penetration test and vulnerability assessments twice a year, using certified testers, and send the executive summary to the central bank within 15 days after the test. | Calendar tasks with deadline | [Regulatory]. New local. Default: On. Basis: RS17 (Reg 50/2022 Art 10) |
| RW-SEC-02 | Multi-factor authentication for remote access | Anyone reaching internal networks from outside must use multi-factor authentication, unless the IT security head approves an equivalent control in writing. | Hard: block remote session | [Regulatory]. Analogue of REG-03. Default: On. Basis: RS17 (Art 16) |
| RW-SEC-03 | Cyber incident report to the central bank | A cyber incident must be reported to the central bank (Reg 50/2022 Art 21; Directive 4230/2024-00037). The hour count and severity test were not found, so the clock is a placeholder and the rule ships off. | Report; clock is a placeholder | [Regulatory]. Param of Nigeria 24h CBN clock (value not found). Default: Off (threshold to be confirmed). Basis: RS17, RS18; hour count not found |
| RW-SEC-04 | Personal-data breach notice within 48 hours | A personal-data breach must reach the National Cyber Security Authority within 48 hours of awareness, and affected people are told when the risk is high. | Legal clock: 48 hours | [Regulatory]. Param of Nigeria 72h NDPC clock (48h in Rwanda). Default: On. Basis: RS14 (Art 43, Art 45 per DLA Piper); ConsentStack |
| RW-SEC-05 | Telecom incident reporting to RURA | Critical and major incidents on telecom networks are reported to RURA immediately; moderate incidents are reported monthly. Mobile-money networks run on licensed telecom networks. | Legal clock: immediate or monthly | [Regulatory]. New local (telecom tenants). Default: On. Basis: RS20 (Art 28) |
| RW-SEC-06 | Telecom security audit and testing cadence | Telecom licensees test annually, assess vulnerabilities twice a year, audit internally twice a year, fix deficiencies within 3 months and file reports within 30 days. | Calendar tasks | [Regulatory]. New local (telecom tenants). Default: On. Basis: RS20 (Arts 20-24, 26) |
| RW-SEC-07 | Data localisation and cross-border transfer authorisation | Personal data is stored outside Rwanda, or sent abroad, without the registration certificate or authorisation the data protection law requires, or without a written contract. | Flag and hold the transfer job | [Regulatory]. New local. Default: On. Basis: RS14 (Arts 48-50 per DLA Piper) |
| RW-SEC-08 | Encryption in transit and at rest | Customer data stores or links are found unencrypted. | Alert | [Regulatory]. New local. Default: On. Basis: RS17 (Art 19) |
| RW-SEC-09 | Third-party and outsourcing assessments | A service provider has no recorded due diligence, minimum security practices or periodic assessment. | Task and readiness check | [Regulatory]. New local. Default: On. Basis: RS17 (Art 14); RS19 (Reg 74/2023 Art 44) |
| RW-SEC-10 | Stolen-password testing (credential stuffing) | Ten or more failed logins from one network address across five or more accounts in 5 minutes. | 30 points | [Industry practice]. Param of SEC-01. Default: On. Basis: Nigeria library |
| RW-SEC-11 | Approval-prompt flooding | Five or more push or OTP prompts to one user in 10 minutes. | 25 points | [Industry practice]. Param of SEC-04. Default: On. Basis: Nigeria library |
| RW-SEC-12 | API key misuse | A key is used from a new network or country, at five times its normal rate, or with many failed requests. | 30 points | [Industry practice]. Param of SEC-08. Default: On. Basis: Nigeria library |
| RW-SEC-13 | Rights raised, then data exported | A user's role is upgraded and they export data within the hour. | 30 points | [Industry practice]. Param of SEC-10. Default: On. Basis: Nigeria library |
| RW-SEC-14 | Unusually large export | An export far above the user's or institution's normal size; a possible personal-data breach that starts the 48-hour clock in RW-SEC-04 once confirmed. | 25 points | [Industry practice]. Param of SEC-12. Default: On. Basis: Nigeria library; RS14 |
| RW-SEC-15 | Evidence preservation after illegal computer use | A service provider that sees illegal use of a computer system reports it to the cybersecurity authority immediately and preserves the evidence. | Task: report and preserve | [Regulatory]. New local. Default: On. Basis: RS15 (Law 60/2018 Arts 5-6, per summary) |
| RW-SEC-16 | Illegal SIM-swap incident report (draft rule) | A telecom operator finds an illegal SIM swap and must report it within 48 hours under a draft RURA regulation. The draft's RWF 10 million swap fine is also unconfirmed. | Report when enabled | [Regulatory]. New local (telecom tenants). Default: Off (verify first). Basis: RS21 (draft, January 2023) |

#### Governance presets (RW-GOV)

| ID | Rule | What we look for (plain English) | Action or points | Tag, lineage, default, basis |
|---|---|---|---|---|
| RW-GOV-01 | Record keeping (10 years) | Customer and transaction records are kept at least 10 years from the end of the business relationship. An older source says 5 years under the repealed 2023 law; the stricter figure is the default. | Deny deletion before the period ends | [Regulatory]. Param of GOV-08. Default: On. Basis: RS2 (Art 21 per FUR); RS6 (5y conflict) |
| RW-GOV-02 | No tipping off | Nothing about an STR or suspicion flag appears in customer-facing screens, API responses, webhooks or exports. Reporters are protected from liability. | Strip fields and log | [Regulatory]. Param of GOV-09. Default: On. Basis: RS3 (Art 34 per map) |
| RW-GOV-03 | Approved reporting to the FIC | Reports are prepared by the system and approved by a compliance officer before submission. FIC provides sector templates; no goAML use was found for Rwanda, so the channel is a tenant setting. | Hold for approval | [Regulatory]. Param of GOV-06. Default: On. Basis: RS6 (no goAML evidence); FIC site |
| RW-GOV-04 | Targeted financial sanctions workflow | A designation starts a 24-hour freeze workflow with named owners, evidence and a log. Missing it costs RWF 1 million to RWF 10 million. | Case with 24-hour timer | [Regulatory]. Param of GOV-05. Default: On. Basis: RS2, RS3 |
| RW-GOV-05 | Data protection registration, officer and impact assessment | Register as controller or processor with the National Cyber Security Authority before processing; appoint a data protection officer where required; run impact assessments for high-risk processing; and list international transfers in the certificate. | Readiness check | [Regulatory]. New local. Default: On. Basis: RS14 (Arts 29, 31, 40 per DLA Piper) |
| RW-GOV-06 | Central bank cyber governance | Board and senior management own cyber security; an IT board committee and a qualified IT security head exist; a written incident response plan is tested; staff training includes phishing simulations; a data retention and disposal policy exists. | Readiness check | [Regulatory]. New local. Default: On. Basis: RS17 (Arts 4, 5, 7, 17, 18, 20); RS18 |
| RW-GOV-07 | Virtual-asset provider licence record | A virtual-asset tenant records its Capital Market Authority licence. Capital, deadlines and article numbers are not yet published, so the check ships off. | Readiness check when enabled | [Regulatory]. New local. Default: Off (threshold to be confirmed). Basis: RS13 |
| RW-GOV-08 | Sector risk ratings from the 2024 risk assessment | Customers and counterparties carry a sector risk tag from the national risk assessment: real estate and bailiffs Medium-High; banks, e-money and forex bureaux Medium; remittance and virtual assets Medium-Low; lawyers and accountants Low. Tenants adjust them. | Risk tag feeds scoring | [Industry practice]. New local. Default: On. Basis: RS25 (NRA 2024, Zigram summary) |
| RW-GOV-09 | Internal service levels | Triage a high-risk alert within 4 hours and reach an STR decision within 12 hours, so a 24-hour STR clock, whatever the final deadline, is never at risk. | Escalate on breach | [My design]. Param of Nigeria recommended SLAs. Default: On. Basis: My design |
| RW-GOV-10 | Retention conflicts between laws | When the 10-year AML retention duty conflicts with a data-protection deletion request or the disposal policy, the longer legal duty wins and the reason is logged. | Deny and log | [My design]. New local. Default: On. Basis: RS2, RS14, RS17 (Art 17) |

### 5.2 WHEN/IF/THEN logic for engineers

#### RW-REG: mandatory controls

```
RULE RW-REG-01 | control=hard | action=flag
WHEN  txn.type IN (cash.deposit, forex.buy, remittance.cash_in)
SCOPE txn.cash_source = cross_border
IF    txn.amount_usd_equiv > tenant.cfg.rw_declaration_threshold       # default USD 10_000; the regulation says amounts not exceeding USD 10_000 need no declaration
AND   declaration.reference IS NULL
THEN  flag=undeclared_cross_border_cash  case=open  task=report.FIC  reason=cross_border_cash
      # declarations reach the FIC within 2 days (Art 9); customs holds cash up to 2 hours (Art 7); those duties sit with the authority, not the tenant

RULE RW-REG-02 | control=hard | action=freeze
WHEN  designation.received OR list.updated
IF    ANY(customer, beneficial_owner, counterparty) MATCHES designation
THEN  freeze=account  deadline=designation.received_at + 24h  task=report.FIC  case=open
      reason=tfs_freeze  gov=RW-GOV-04

RULE RW-REG-03 | control=hard | action=block
WHEN  txn.created OR customer.onboarded OR list.updated
IF    screen(party.name, party.id, party.account) MATCHES lists(un_consolidated, rw_designations, tenant_lists)
        USING fuzzy + transliteration  score >= tenant.cfg.match_threshold
AND   match.reviewed = confirmed OR tenant.cfg.auto_block_on_strong_match = true
THEN  decision=block  case=open  trigger=RW-REG-02  reason=tfs_match

RULE RW-REG-04 | control=hard | action=report
WHEN  alert.suspicion_recorded OR alert.suspicion_confirmed
THEN  task=report.STR  due=tenant.cfg.rw_str_due        # PLACEHOLDER 24h from suspicion [My design]; statutory deadline not found: threshold to be confirmed
      to=FIC  gov=RW-GOV-02, RW-GOV-03
      # the duty is live; only the hour count is unconfirmed

RULE RW-REG-05 | control=hard | action=report
WHEN  txn.type IN (cash.deposit, cash.withdrawal, cash.payment, transfer.wire)
SCOPE tenant.cfg.rw_ctr_enabled = true
IF    txn.amount >= tenant.cfg.rw_ctr_threshold       # PLACEHOLDER RWF 10_000_000 (from the brief, not found in any source); threshold to be confirmed; disabled-by-default
THEN  task=report.CTR  due=tenant.cfg.rw_ctr_due  to=FIC

RULE RW-REG-06 | control=hard | action=edd
WHEN  customer.onboarded OR customer.periodic_review OR txn.created
IF    customer.is_pep = true OR ANY(beneficial_owner.is_pep = true)
THEN  decision=review  require=edd,source_of_funds,source_of_wealth,senior_approval  reason=pep

RULE RW-REG-07 | control=hard | action=decline
WHEN  txn.type = p2p.interoperable
SCOPE tenant.type IN (bank, emi, psp) AND txn.created_at >= 2026-07-14
IF    txn.amount > MIN(tenant.cfg.rw_ekash_cap, tenant.cfg.own_limit)      # default RWF 10_000_000
THEN  decision=decline  reason=ekash_cap
IF    txn.fee > tenant.cfg.rw_ekash_fee_max                               # default RWF 20
THEN  block_fee  reason=ekash_fee_ceiling
      # Directive text not seen; aggregator source only: verify before enabling for production

RULE RW-REG-08 | control=hard | action=decline
WHEN  txn.type = cash.withdrawal
SCOPE tenant.type = bank AND tenant.cfg.rw_cash_ceiling_enabled = true
WINDOW 1d BY customer.id
IF    SUM(txn.amount) + txn.amount > tenant.cfg.rw_cash_ceiling       # default RWF 5_000_000; verify; disabled-by-default
UNLESS exception.authorised_by_bank = true
THEN  decision=decline  route=credit_transfer
      IF exception.authorised_by_bank = true THEN add_to=weekly_bnr_exception_report

RULE RW-REG-09 | control=hard | action=decline
WHEN  wallet.open
SCOPE tenant.type = emi
IF    COUNT(wallet WHERE customer.nida_id = this AND issuer = tenant.id) >= tenant.cfg.rw_max_emoney_accounts      # default 3
AND   customer.bnr_authorisation IS NULL
THEN  decision=decline  reason=max_emoney_accounts       # scope per issuer is my reading of Art 9

RULE RW-REG-10 | control=hard | action=block
WHEN  account.open OR wallet.open OR kyc.refresh
IF    identity.nida_verified != true OR customer.name IS NULL
THEN  decision=block  reason=nida_verification_missing

RULE RW-REG-11 | control=hard | action=decline
WHEN  txn.created
SCOPE tenant.type = emi AND txn.channel IN (mobile, ussd, web)
IF    txn.auth_method IS NULL
THEN  decision=decline  reason=no_customer_authentication

RULE RW-REG-12 | control=hard | action=review
WHEN  customer.onboarded OR customer.periodic_review
SCOPE customer.type = legal_person
IF    beneficial_owner.identified = false OR beneficial_owner.documents_complete = false
      # ownership share that defines a beneficial owner: tenant.cfg.rw_bo_pct, default NULL (not found): threshold to be confirmed
THEN  decision=review  restrict=new_products  reason=beneficial_owner_missing

RULE RW-REG-13 | control=hard | action=review
WHEN  txn.created
IF    ( counterparty.type = virtual_asset_provider AND counterparty.cma_licensed != true )
   OR ( merchant.accepts_virtual_assets_as_payment = true AND merchant.bnr_authorisation IS NULL )
THEN  decision=review  hold=true  reason=unlicensed_virtual_asset_activity  link=CRY-03
      # licence register must be loaded by the tenant; article numbers of Law 023/2026 not found

RULE RW-REG-14 | control=hard | action=hold
WHEN  crypto.transfer
SCOPE tenant.type = vasp AND tenant.cfg.rw_travel_rule_enabled = true
IF    txn.amount_usd_equiv >= tenant.cfg.rw_travel_rule_min          # PLACEHOLDER NULL: threshold to be confirmed; disabled-by-default
AND   ANY OF ( originator.data_complete = false , beneficiary.data_complete = false )
THEN  decision=hold  link=CRY-13
```

#### RW-TM: typology and monitoring

```
RULE RW-TM-01 | weight=20
WHEN  txn.created
SCOPE account.age < 7d
IF    SUM(txn.amount OVER account.age) > 10 * kyc_tier.norm(account.kyc_tier)
THEN  signal=RW-TM-01  evidence=(volume, tier_norm)

RULE RW-TM-02 | weight=25 | action=step_up
WHEN  txn.type IN (transfer.outbound, wallet.send, wallet.cash_out) OR otp.requested
IF    ANY OF ( telco.sim_swap_flag = true , EXISTS(sim.replaced OVER tenant.cfg.rw_sim_swap_window) )     # default 72h; not a legal clock
AND   txn.amount > tenant.cfg.floor                    # suggested RWF 200_000 (analyst judgement)
THEN  signal=RW-TM-02  evidence=(swap_time, channel)

RULE RW-TM-03 | weight=15
WHEN  sim.registered OR sim.replaced
WINDOW 30d BY nida_id
IF    COUNT(sim.registered OR sim.replaced) >= 2
OR    COUNT(DISTINCT msisdn WHERE operator = same) > tenant.cfg.rw_sims_per_mno     # default 3 (draft only); disabled-by-default part
THEN  signal=RW-TM-03

RULE RW-TM-04 | weight=20 | action=review
WHEN  txn.type IN (wallet.credit, transfer.inbound)
IF    ANY OF ( identity.sim_nida_verified = false , msisdn.country != RW )
AND   txn.amount > tenant.cfg.high_value             # suggested RWF 1_000_000
THEN  signal=RW-TM-04

RULE RW-TM-05 | weight=30 | action=review
WHEN  txn.type IN (transfer.outbound, wallet.send)
WINDOW 30m BY account.id
IF    EXISTS(device.bound WHERE device.is_new = true)
AND   EXISTS(beneficiary.added WHERE beneficiary.is_new = true)
AND   txn.amount > tenant.cfg.floor
THEN  signal=RW-TM-05  evidence=(device.id, beneficiary.id, times)

RULE RW-TM-06 | weight=25 | action=step_up
WHEN  txn.type IN (transfer.outbound, wallet.send)
IF    ANY OF ( EXISTS(profile.password_changed OVER 24h) , EXISTS(profile.pin_changed OVER 24h) ,
               EXISTS(profile.phone_changed OVER 24h) , EXISTS(profile.email_changed OVER 24h) )
AND   txn.amount > tenant.cfg.floor
THEN  signal=RW-TM-06

RULE RW-TM-07 | weight=25 | action=review
WHEN  txn.type IN (transfer.outbound, wallet.send)
WINDOW 60m BY account.id
IF    SUM(txn.amount WHERE direction=out) >= 0.80 * SUM(txn.amount WHERE direction=in)
AND   SUM(txn.amount WHERE direction=in) > tenant.cfg.floor
THEN  signal=RW-TM-07  evidence=(in_sum, out_sum, ratio)

RULE RW-TM-08 | weight=20
WHEN  txn.type = transfer.inbound
WINDOW 24h BY account.id
IF    COUNT(DISTINCT sender.id) >= 10
THEN  signal=RW-TM-08

RULE RW-TM-09 | weight=20
WHEN  txn.type IN (transfer.outbound, wallet.send)
WINDOW 60m BY source_inflow.id
IF    COUNT(DISTINCT beneficiary.id) >= 5
THEN  signal=RW-TM-09

RULE RW-TM-10 | weight=20
WHEN  txn.created
IF    account.days_since_last_activity >= 90
AND   txn.amount > tenant.cfg.large_after_dormancy     # suggested RWF 1_000_000
THEN  signal=RW-TM-10

RULE RW-TM-11 | weight=25 | action=review
WHEN  account.created OR application.submitted OR txn.created
IF    ANY OF ( COUNT(DISTINCT account.id BY device.id) >= 3 ,
               COUNT(DISTINCT account.id BY nida_id WHERE issuer = tenant.id) > tenant.cfg.rw_max_emoney_accounts ,
               COUNT(DISTINCT account.id BY phone) >= 2 , COUNT(DISTINCT account.id BY address) >= 2 )
THEN  signal=RW-TM-11  evidence=(shared_identifier, linked_accounts)

RULE RW-TM-12 | weight=25 | action=review
WHEN  txn.created
WINDOW 24h BY account.id
IF    COUNT(txn WHERE txn.amount BETWEEN 0.90*L AND 0.999*L) >= 3
      WHERE L IN ( tenant.cfg.rw_cash_ceiling , tenant.cfg.rw_ekash_cap , tenant.cfg.own_limit )
THEN  signal=RW-TM-12  evidence=(limit_hit, amounts)

RULE RW-TM-13 | weight=20
WHEN  txn.type IN (transfer.outbound, wallet.send)
WINDOW 24h BY account.id
IF    SUM(txn.amount WHERE beneficiary.is_new = true) >= tenant.cfg.rw_new_payee_cum      # default RWF 1_000_000 (suggested band RWF 500_000 to 1_000_000)
THEN  signal=RW-TM-13

RULE RW-TM-14 | weight=20
WHEN  txn.type IN (transfer.outbound, wallet.send)
WINDOW 24h BY account.id
IF    SUM(txn.amount) > 3 * AVG(daily_outbound OVER 30d)
AND   SUM(txn.amount) > tenant.cfg.floor
THEN  signal=RW-TM-14
IF    txn.amount > AVG(txn.amount OVER 90d) + 3 * STDDEV(txn.amount OVER 90d) AND customer.history_days >= 30
THEN  signal=RW-TM-14 weight=24

RULE RW-TM-15 | weight=10 or 20
WHEN  session.login OR txn.created
IF    geo.country NOT IN profile.usual_countries THEN signal=RW-TM-15 weight=10
IF    distance(geo, previous.geo) / time_between > 900 km/h THEN signal=RW-TM-15 weight=20

RULE RW-TM-16 | weight=25 | action=review
WHEN  agent.daily_job OR txn.type = wallet.cash_out
SCOPE agent.active = true
WINDOW 60m BY agent.id
IF    COUNT(DISTINCT customer.id WHERE txn.type = wallet.cash_out) >= tenant.cfg.rw_agent_cluster_min
OR    agent.float_topup_24h > tenant.cfg.float_peer_mult * peer_group.p95(float_topup_24h)
      # PLACEHOLDER values: threshold to be confirmed; disabled-by-default
THEN  signal=RW-TM-16  visibility=compliance_only

RULE RW-TM-17 | weight=25 | action=review
WHEN  txn.type = wallet.cash_out
WINDOW 72h BY account.id
IF    EXISTS(inflow WHERE inflow.type = remittance.international.in AND inflow.amount >= tenant.cfg.floor)
AND   COUNT(DISTINCT agent.id) >= 3
AND   SUM(txn.amount) >= 0.70 * inflow.amount
THEN  signal=RW-TM-17

RULE RW-TM-18 | weight=25 | action=review
WHEN  txn.type IN (transfer.outbound, cheque.issue)
SCOPE counterparty.sector = real_estate OR memo MATCHES (plot, land, house, property, developer)
IF    txn.amount >= tenant.cfg.rw_realestate_floor                 # PLACEHOLDER NULL: threshold to be confirmed; disabled-by-default
AND   ANY OF ( funding.cash_share_30d >= 0.50 , account.age < 90d , txn.amount > 5 * customer.declared_monthly_income )
THEN  signal=RW-TM-18  require=source_of_funds

RULE RW-TM-19 | weight=30 | action=review
WHEN  txn.created
SCOPE customer.type = legal_person
IF    COUNT(shell_indicators WHERE ind IN (no_employees, shared_address, shared_director, no_website, beneficial_owner_unclear, high_pass_through)) >= 3
THEN  signal=RW-TM-19  evidence=(indicators)

RULE RW-TM-20 | weight=20 or 30 | action=review
WHEN  txn.created
SCOPE account.linked_to_employee = true OR staff_action.exists
IF    SUM(txn.amount OVER 30d) > peer_group.p95(SUM OVER 30d)                       THEN signal=RW-TM-20 weight=20
IF    EXISTS(staff_action WHERE type IN (view_contact, change_contact, change_limits) OVER 24h) AND txn.type = transfer.outbound
                                                                                    THEN signal=RW-TM-20 weight=30
      visibility=compliance_only

RULE RW-TM-21 | weight=25 | action=review
WHEN  txn.type = transfer.inbound
WINDOW 7d BY account.id
IF    COUNT(DISTINCT sender.id WHERE unrelated = true) >= 20
AND   SUM(outflow WHERE recipient IN prior_senders) >= 0.30 * SUM(outflow)
THEN  signal=RW-TM-21

RULE RW-TM-22 | weight=25 | action=review
WHEN  txn.type IN (transfer.outbound, wallet.send)
WINDOW 7d BY account.id
IF    COUNT(DISTINCT sender.id) >= 10
AND   SUM(txn.amount WHERE counterparty.tag IN (vasp, p2p_crypto, unlicensed_vasp)) >= 0.70 * SUM(txn.amount WHERE direction=in)
THEN  signal=RW-TM-22  link=CRY-01,CRY-03,CRY-08

RULE RW-TM-23 | weight=25 | action=review
WHEN  txn.type IN (cash.deposit, forex.buy)
SCOPE txn.cash_source = cross_border
WINDOW 7d BY customer.id
IF    COUNT(txn WHERE txn.amount_usd_equiv BETWEEN 0.90*tenant.cfg.rw_declaration_threshold AND 0.999*tenant.cfg.rw_declaration_threshold) >= 2
THEN  signal=RW-TM-23

RULE RW-TM-24 | weight=20 | action=review
WHEN  cdr.daily_job
SCOPE tenant.type = telco
IF    COUNT(DISTINCT sim.id BY device.imei OR gateway.id) >= tenant.cfg.rw_simbox_sims        # default 30
OR    international_inbound_terminating_as_local_ratio > tenant.cfg.simbox_ratio
THEN  signal=RW-TM-24
```

#### RW-SEC: cybersecurity

```
RULE RW-SEC-01 | control=clock | action=task
SCOPE tenant.type IN (bank, mfi, insurer, emi, psp) AND tenant.bnr_regulated = true
WHEN  pentest.last_done > 12mo           THEN task=run.pentest
WHEN  vuln_assessment.last_done > 6mo    THEN task=run.vulnerability_assessment
WHEN  pentest.completed
IF    tester.certification NOT IN (CISSP, CISM, CISA, CEH, OSCP, LPT, equivalent) THEN flag=uncertified_tester
THEN  task=send.BNR_executive_summary  due=15d_after_test

RULE RW-SEC-02 | control=hard | action=block
WHEN  remote.access.login
SCOPE tenant.bnr_regulated = true
IF    session.mfa_passed = false
UNLESS compensating_control.approved_in_writing_by = it_security_head
THEN  decision=block  reason=mfa_required_remote_access

RULE RW-SEC-03 | control=clock | action=report
WHEN  incident.detected
SCOPE tenant.bnr_regulated = true
IF    incident.severity IN tenant.cfg.rw_bnr_reportable_levels      # PLACEHOLDER: severity test not found
THEN  task=report.BNR  due=tenant.cfg.rw_bnr_incident_clock         # PLACEHOLDER NULL: threshold to be confirmed; disabled-by-default; obtain from BNR supervisor

RULE RW-SEC-04 | control=clock | action=report
WHEN  breach.confirmed
IF    breach.personal_data = true
THEN  task=notify.NCSA  due=48h_from_awareness
IF    breach.high_risk = true
THEN  task=notify.data_subjects  due=tenant.cfg.rw_subject_notice      # default 48h (Hogan Lovells); DLA Piper gives no hour count

RULE RW-SEC-05 | control=clock | action=report
WHEN  incident.detected
SCOPE tenant.type = telco OR tenant.rura_licensed = true
IF    incident.class IN (critical, major)  THEN task=report.RURA  due=immediate     # no hour count in the regulation
IF    incident.class = moderate            THEN task=report.RURA  due=monthly

RULE RW-SEC-06 | control=clock | action=task
SCOPE tenant.type = telco OR tenant.rura_licensed = true
WHEN  pentest.last_done > 12mo         THEN task=run.pentest
WHEN  vuln_assessment.last_done > 6mo  THEN task=run.vulnerability_assessment
WHEN  internal_audit.last_done > 6mo   THEN task=run.internal_audit
WHEN  audit.finding_opened             THEN task=remediate  due=3mo
WHEN  audit.completed                  THEN task=send.report_to_RURA  due=30d

RULE RW-SEC-07 | control=hard | action=flag
WHEN  data.transfer OR storage.region_changed
SCOPE data.class = personal AND destination.country != RW
IF    NOT ( ncsa.certificate.authorises_external_storage = true OR ncsa.transfer_authorisation.valid = true OR subject_consent.recorded = true )
OR    contract.written = false
THEN  flag=localisation_or_transfer  hold=transfer_job  reason=data_localisation

RULE RW-SEC-08 | control=hard | action=alert
WHEN  config.scan
SCOPE tenant.bnr_regulated = true
IF    ANY OF ( datastore.encrypted_at_rest = false , link.tls_enforced = false )
THEN  alert=encryption_gap  severity=high

RULE RW-SEC-09 | control=policy | action=task
WHEN  vendor.onboard OR vendor.annual_review
SCOPE tenant.bnr_regulated = true
IF    vendor.due_diligence.done = false OR vendor.last_assessment > tenant.cfg.rw_vendor_cycle      # default 12mo
THEN  task=assess_vendor  readiness=fail

RULE RW-SEC-10 | weight=30
WHEN  auth.failure
WINDOW 5m BY ip
IF    COUNT(auth.failure) >= 10 AND COUNT(DISTINCT account.id) >= 5
THEN  signal=RW-SEC-10  action_hint=rate_limit_ip

RULE RW-SEC-11 | weight=25
WHEN  mfa.prompt_sent
WINDOW 10m BY account.id
IF    COUNT(mfa.prompt_sent WHERE type IN (push, otp)) >= 5
THEN  signal=RW-SEC-11  action_hint=pause_prompts

RULE RW-SEC-12 | weight=30
WHEN  api.request
SCOPE auth.type = api_key
IF    ANY OF ( request.network NOT IN key.known_networks , request.country NOT IN key.known_countries ,
               RATE(request OVER 5m) > 5 * key.baseline_rate , RATIO(status IN (400, 404) OVER 10m) > tenant.cfg.probe_ratio )
THEN  signal=RW-SEC-12  action_hint=throttle_key

RULE RW-SEC-13 | weight=30
WHEN  data.export
WINDOW 1h BY user.id
IF    EXISTS(user.role_changed)
THEN  signal=RW-SEC-13

RULE RW-SEC-14 | weight=25
WHEN  data.export
IF    export.rows > 5 * BASELINE(export.rows BY user.id OVER 30d)
   OR export.rows > 5 * BASELINE(export.rows BY tenant.id OVER 30d)
THEN  signal=RW-SEC-14  link=RW-SEC-04

RULE RW-SEC-15 | control=clock | action=report
WHEN  incident.classified = illegal_computer_use
THEN  task=report.NCSA  due=immediate  preserve=logs,evidence  retention=case_closed_plus(tenant.cfg.evidence_hold)

RULE RW-SEC-16 | control=clock | action=report
WHEN  incident.classified = illegal_sim_swap
SCOPE tenant.type = telco AND tenant.cfg.rw_sim_draft_enabled = true
THEN  task=report.RURA  due=48h      # draft only: disabled-by-default until a final regulation is confirmed
```

#### RW-GOV: governance

```
RULE RW-GOV-01 | type=control
WHEN  record.delete_requested
IF    record.age < tenant.cfg.rw_retention_years after relationship_end      # default 10y; 5y reading under Law 028/2023
THEN  deny_delete  reason=retention

RULE RW-GOV-02 | type=control
WHEN  api.response OR webhook.send OR export.generate
IF    payload CONTAINS (str.exists OR str.status OR flag.visibility = compliance_only)
AND   recipient.role NOT IN (compliance_officer, mlro)
THEN  strip_fields  AND  log=audit

RULE RW-GOV-03 | type=workflow
WHEN  report.draft_ready (STR, CTR, declaration)
IF    report.approved_by IN roles(compliance_officer)
THEN  allow_submit  via=tenant.cfg.fic_channel          # goAML not confirmed
ELSE  hold_for_approval

RULE RW-GOV-04 | type=workflow
WHEN  designation.received
THEN  case=open  owner=mlro  timer=24h  steps=(screen, freeze, report_FIC, confirm)
IF    NOW > designation.received_at + 24h AND case.freeze_done = false THEN escalate=board  reason=tfs_deadline_breach

RULE RW-GOV-05 | type=job | schedule=daily
IF    tenant.ncsa_certificate IS NULL OR tenant.ncsa_certificate.expired = true  THEN readiness=fail reason=ncsa_registration
IF    tenant.dpo IS NULL AND tenant.cfg.rw_dpo_required = true                   THEN readiness=fail reason=dpo_missing
IF    processing.high_risk = true AND dpia.completed = false                     THEN readiness=fail reason=dpia_missing
IF    transfer.international.exists AND NOT ncsa_certificate.lists_transfer      THEN readiness=fail reason=transfer_not_listed

RULE RW-GOV-06 | type=policy
SCOPE tenant.bnr_regulated = true
IF    ANY OF ( board.cyber_owner IS NULL , it_security_head IS NULL , incident_response_plan.tested_last_12mo = false ,
               training.phishing_simulation_last_12mo = false , retention_disposal_policy IS NULL )
THEN  readiness=fail  reason=bnr_cyber_governance

RULE RW-GOV-07 | type=policy
WHEN  tenant.activate
SCOPE tenant.type = vasp AND tenant.cfg.rw_vasp_check_enabled = true
IF    tenant.cma_licence IS NULL
THEN  deny_activate  reason=missing_cma_licence      # disabled-by-default: implementing regulations pending

RULE RW-GOV-08 | type=policy
WHEN  customer.onboarded OR customer.periodic_review
THEN  customer.sector_risk = tenant.cfg.rw_sector_risk[customer.sector]
      # default: real_estate MH, bailiffs MH, banking M, e_money M, forex M, remittance ML, vasp ML, legal_accounting L

RULE RW-GOV-09 | type=sla
WHEN  alert.created
IF    alert.risk = high AND NOW - alert.created > 4h AND alert.triaged = false   THEN escalate
IF    alert.suspicion_recorded AND NOW - alert.suspicion_start_ts > 12h AND alert.str_decision IS NULL   THEN escalate

RULE RW-GOV-10 | type=control
WHEN  deletion.requested
IF    record.under_legal_hold = true OR record.age < tenant.cfg.rw_retention_years after relationship_end
THEN  deny_delete  log=reason  notify=dpo      # design choice: confirm with Rwandan counsel
```

## 6. Legal clocks and thresholds side by side

Nigerian values come from the Nigeria library (sections 2, 5 and 8); Ghanaian and Rwandan values come from the notes. "Not found" means no readable source gave a number. The last column is a working default for a multi-country RiskOS tenant; every default is configurable per country, and each rule should evaluate against the jurisdiction that governs the customer or the booking entity, not the strictest of all three.

| Item | Nigeria | Ghana | Rwanda | Working default for a multi-country tenant |
|---|---|---|---|---|
| STR clock | Screen alerts within 72 hours; file within 24 hours of confirmed suspicion (REG-14); older summaries say 24 hours from the transaction | 24 hours from forming suspicion (Act 1044 s.38); BoG guideline says from confirmation (GH-REG-06) | Duty in Art 32; numeric deadline not found (RW-REG-04) | Start a 24-hour clock at the earliest recorded suspicion in all three countries; label Rwanda "design default" until the Gazette text is read; internal SLA of 4 hours triage and 12 hours decision |
| Cash transaction report | Domestic cash of NGN 5m (individual) or NGN 10m (corporate) via goAML (REG-17) | GHS 50,000 banks and securities; GHS 20,000 specialised deposit-taking and forex bureaux (GH-REG-04) | Not found; RW-REG-05 off with a placeholder | Per-country thresholds; keep Rwanda off until confirmed |
| Cross-border report | US$10,000 or more, within 1 day (REG-10) | US$1,000 or more, within 24 hours of the transfer (GH-REG-05) | Cash declaration above US$10,000 (RW-REG-01); no wire-report threshold found | Evaluate by origin and destination country; use Ghana's lower US$1,000 only for Ghana-booked transfers |
| Record retention | 5 years; some summaries say 7 (GOV-08) | 5 years; 6 years for payment-platform audit logs (GH-GOV-01, GH-SEC-11) | 10 years after the relationship ends; 5 years under the old law (RW-GOV-01) | Per-country minimums; keep the longest applicable period for shared records and log the reason when data-protection deletion is denied (RW-GOV-10) |
| Personal-data breach notice | 72 hours to the NDPC | "As soon as reasonably practicable" to the DPC; no hour count (GH-SEC-12) | 48 hours to NCSA (RW-SEC-04) | 48 hours internal clock where Rwandan data subjects are affected; 72 hours otherwise, labelled policy for Ghana |
| Cyber incident to financial regulator or agency | 24 hours to CBN; 72 hours to national or sector CERT | CSA 24 hours (designated CII only); BoG within 10 days under Act 987; BoG CISD 2026 clock not found (GH-SEC-08, GH-SEC-09) | BNR Art 21 hour count not found (RW-SEC-03); RURA immediate for critical or major telecom incidents; NCSA no coded deadline | Open every incident with all applicable clocks; run the shortest known clock (24 hours) for banks and payment institutions; leave unknown clocks null |
| Freeze and holds | "Immediate" with no hour count; NFIU or EFCC stop order up to 72 hours | Without prior notice; FIC suspension up to 7 working days; court application within 7 working days (E.I. 2 summary says 48 hours) (GH-REG-08, GH-REG-11) | Within 24 hours (Art 2(b), Art 4) (RW-REG-02); 3 working days under the old law | 24-hour freeze SLA everywhere; stop orders and suspensions carry automatic expiry dates |
| Wallet limits | Tier limits conflict, none hard-coded; new-device cap NGN 20,000 for 24 hours (REG-01) | Daily GHS 3,000, 15,000, 25,000; balance GHS 5,000, 40,000, 75,000; monthly GHS 10,000 (Minimum) (GH-REG-01 to 03); no new-device cap found | Issuer-defined (Art 36); eKash RWF 10 million per transaction and RWF 20 fee (RW-REG-07) | Per-issuer configuration; seed Ghana from the BoG figures and Rwanda from the issuer's own limits |
| Cash payment or withdrawal ceiling | NGN 5m individual, NGN 10m corporate payment ceiling (REG-07); weekly NGN 1.5m withdrawal charge (REG-08) | None found beyond CTR | RWF 5 million per day (2009 directive; verify) (RW-REG-08) | Off for Rwanda until verified |
| Sanctions and TFS | REG-11 block, freeze and report | UN lists and Ghana designations; freeze without prior notice; quarterly reports and drills (GH-REG-08, GH-GOV-06) | Freeze within 24 hours; fine RWF 1 to 10 million (RW-REG-02, RW-GOV-04) | One screening engine, per-country list sets and clocks |
| PEPs | REG-12, REG-18 | Senior approval; source of wealth; transactions reported to FIC; 2-year review (GH-REG-07) | Senior approval and source of wealth (RW-REG-06) | EDD in all three; keep per-country report outputs |
| Penetration testing and vulnerability work | Annual self-assessment; quarterly board reports with test results | CISD 2026 cadence not found | Annual pentest, vulnerability assessment twice a year, summary to BNR in 15 days (RW-SEC-01) | Annual pentest and twice-yearly assessments everywhere |
| Outsourcing and data location | Third-party audit rights and standards | Notice before outsourcing; SLA within 10 days (GH-SEC-10); transfer needs consent and safeguards (GH-SEC-13) | Provider assessments (Art 14); personal data stored in Rwanda by default (RW-SEC-07) | Treat Rwanda data localisation as a hard default; flag Ghana transfers for consent and safeguards |
| Penalty basis | Naira amounts | Penalty units at GHS 12 (inference) | RWF amounts | Show the unit and the currency estimate per country |

## 7. Gaps, conflicts and sources

Every conflict and unverified item from the notes appears below with how the library treats it. Nothing here was resolved by assumption unless the treatment column says so.

### 7.1 Conflicts

| # | Conflict | What the notes show | Treatment in the library |
|---|---|---|---|
| C1 | Rwanda STR numeric deadline | Not found in any readable source; 2024 MER states no statutory timeframe | RW-REG-04 ships with a 24-hour design placeholder, labelled "threshold to be confirmed" |
| C2 | Rwanda cash and wire report threshold | Not found; the RWF 10 million figure is from the brief only; 2024 MER does not mention CTR or wire reporting | RW-REG-05 off, placeholder RWF 10,000,000 labelled as unconfirmed |
| C3 | Rwanda retention | 10 years (Art 21, ESAAMLG August 2025) versus 5 years (2024 MER, old law) | 10 years default; configurable; confirm in the Art 21 text |
| C4 | Ghana CTR | GHS 50,000 (FIC, BoG) versus a vendor claim of GHS 10,000 single and GHS 30,000 cumulative, STR in 3 working days, 7-year retention | Official figures used; vendor claim ignored; the Gazette notice that sets thresholds under ss.40-42 was not found |
| C5 | BoG CISD 2026 incident clock | Launched 30 March 2026; no hours, MFA or pentest cadence found | GH-SEC-09 uses the 10-day Act 987 limit with a null faster-clock placeholder |
| C6 | Law 028/2023 superseded | Law 001/2025 replaces it; article numbers may have been renumbered | Rules cite 001/2025; 2023 article numbers appear only as predecessor references |
| C7 | Law numbering (Rwanda) | Brief says 060/2021; sources show data protection is Law 058/2021, payment system is Law 061/2021 (14 October 2021) | Library uses 058/2021 and 061/2021; cybercrime is Law 60/2018, and no Law 026/2022 was found |
| C8 | Data protection penalties | Ghana: Mondaq 150 and 5,000 penalty units versus Templars GHS 1,500 and GHS 60,000 (150 x 12 = GHS 1,800). Rwanda: RWF 2 to 5 million or 1% of turnover (admin) and 5% (criminal) versus ConsentStack maximum RWF 5 million | Not coded as thresholds; penalty register shows both |
| C9 | Act 1154 commencement | Afriwise says effective in 2025; GNA says enacted December 2025 with full operation by 2027; BoG pages due end Q3 2026 | VASP rules treat VASPs as accountable institutions; GH-REG-13 and GH-GOV-11 off |
| C10 | BNR Directive 45/2026 | Aggregator only; implementation 14 July 2026; RBA dates the directive 9 January 2026 | RW-REG-07 [Regulatory] but flagged "verify before enabling" |
| C11 | Ghana E-Levy | Parliament approved repeal on 26 March 2025; effective date unverified (one source suggests a short-term carry-over) | No levy-avoidance rule |
| C12 | Wallet monthly caps | Medium and Enhanced "unchanged, figure not stated" versus "unrestricted"; a 2026 blog gives much lower estimates (Kolonell) | Medium and Enhanced left null; BoG figures kept; blog treated as unreliable |
| C13 | E.I. 2 court period | E.I. 2 summary says 48 hours; Act 1044 s.56 summary says 7 working days | GH-REG-11 uses 7 working days; unresolved |
| C14 | Ghana STR clock start | Statute: from forming suspicion; BoG guideline: from confirmation | Clock starts at the earliest recorded suspicion |
| C15 | GIABA evaluation | One note says "second mutual evaluation scheduled 2026", another says "third-round" onsite 26 January to 6 February 2026 | Not rule-relevant; round number unresolved |
| C16 | FIC tax typology date | Described as Aug 2025 in one note and January 2026 in another; same PDF | Cited by URL only |
| C17 | Virtual-asset licence categories (Rwanda) | PwC says 12; Andersen says nine "primary" | Not coded; categories pending regulations |
| C18 | NRA outputs | 575 bank STRs led to 11 ML cases and 2 convictions, versus 44 ML cases and 9 convictions in another summary; "FRW 5 million" crime-proceeds threshold has unclear context | Not used in any rule |
| C19 | Rwanda fraud trend | Headline says a 60% fall; figures imply 40% | Not used in any rule |
| C20 | Act 1038 dates | regulations.ai gives 2023 milestones that conflict with the CSA's 31 January 2026 licensing enforcement | Use the 2026 date |

### 7.2 Unverified or unfound items

1. Primary text of Act 1044 s.38 and s.48, Act 987 section numbers, Law 001/2025 article numbers and Law 058/2021 article numbers (all per summary, aggregator or one secondary source).
2. The value of a Ghana penalty unit (GHS 12) and its effective date; Anti-Terrorism Act amendment content; goAML registration for Ghana and Rwanda (a vendor claims goAML for Ghana; none found for Rwanda).
3. Ghana Act 1154 Travel Rule threshold, penalties, capital and gazette date; Rwanda Law 023/2026 article numbers, Travel Rule threshold, capital and transition deadline.
4. Ghana BoG e-money and agent guideline numbers, float limits and Ghana Card as sole ID outside forex bureaux; Rwanda e-money and agent limits and KYC directive.
5. Whether Ghana's Cybersecurity Amendment Bill, Data Protection Bill 2025 and any CSA directive for financial institutions passed; whether the CII 24-hour clock covers all financial institutions or only designated owners.
6. Whether FIC Regulation 002/FIC/2021 survives Law 001/2025; the Law 001/2025 freeze duration (Art 6); tipping-off penalty amounts; FIC and BNR red-flag lists (none found for Rwanda).
7. Rwanda BNR Reg 50/2022 Art 21 hour count and Art 25 amounts; Directive 4230/2024-00037 timelines; CII designation under NCSA; RURA SIM draft adoption; whether Directive 02/2009 is still current; whether the three-account cap is per issuer or across issuers.
8. Rwanda grey-list status in October 2026 (not found either way); next GIABA and ESAAMLG reporting dates.
9. Typology evidence is qualitative for Ghana wrong-transfer scams, interoperability fraud, agent float, galamsey and Rwanda agent clustering, real estate and loan apps; those rules are marked off or flagged [My design].
10. Fetched pages were model summaries and several regulator sites (bnr.rw, ncsa.gov.rw, rura.rw, cbn, esaamlg.org) were not readable directly.

### 7.3 Sources

**Ghana**

- GS1: [FIC report types](https://fic.gov.gh/report-types.html)
- GS2: [BoG AML/CFT&P Guideline, December 2022 (hosted copy)](https://www.moneylaundering.com/wp-content/uploads/2022/12/Ghana.Guidance.AMLCTFGUIDELINe.122322.pdf)
- GS3: [Act 1044 on GhaLII](https://ghalii.org/akn/gh/act/2020/1044/eng@2020-12-29)
- GS4: [RegAlert Act 1044 text](https://regalert.today/document/64b40107-da3b-41fa-9957-3c725fb7b088/text)
- GS5: [SEC administrative sanctions schedule](https://sec.gov.gh/wp-content/uploads/Anti-Money-Laundering/AML_CFT_CPF_Administrative%20Sanctions_Penalties.pdf)
- GS6: [GRA penalty unit page](https://gra.gov.gh/?p=6244)
- GS7: [FIC Executive Instrument 2](https://www.fic.gov.gh/assets/publications/E.-I.-2.pdf); [Anti-Terrorism Act 2008](https://www.fic.gov.gh/assets/publications/Anti-Terrorism-Act-2008-Act-762.pdf)
- GS8: [Graphic on MoMo limits](https://www.graphic.com.gh/business/business-news/see-the-new-momo-balance-transaction-limits-effective-march-1.html); [Graphic, limits reviewed](https://graphic.com.gh/business/business-news/transaction-limits-for-mobile-money-wallets-reviewed.html)
- GS9: [Graphic on forex bureaux rules](https://www.graphic.com.gh/business/business-news/no-ghana-card-no-forex-see-the-new-strict-rules-for-forex-bureaux-to-curb-money-laundering.html)
- GS10: [Kolonell wallet limits blog (conflicting, unreliable)](https://kolonell.com/en/blog/mtn-momo-wallet-limits-ghana-2026)
- GS11: [YouVerify bank AML guide (unverified)](https://youverify.co/en/blogs/aml-compliance-banks-ghana); [YouVerify transaction monitoring (unverified)](https://youverify.co/blog/transaction-monitoring-for-payment-service-banks-in-ghana-fatf-bog-requirements-2026)
- GS12: [SEC press release on the VASP Bill](https://sec.gov.gh/press-release-passage-of-the-virtual-asset-service-providers-bill/)
- GS13: [BoG virtual assets page](https://www.bog.gov.gh/virtual-assets/)
- GS14: [CryptoSlate on Act 1154](https://cryptoslate.com/crypto-laws/ghana-virtual-asset-service-providers-act-2025-act-1154/)
- GS15: [GNA, operationalise by 2027](https://gna.org.gh/2026/08/ghana-to-fully-operationalise-virtual-asset-law-by-2027/)
- GS16: [Afriwise](https://www.afriwise.com/blog/ghanas-vasp-act); [Ndowuona](https://www.ndowuona.com/ghana-vasp-act)
- GS17: [Mondaq on VASP licensing](https://www.mondaq.com/fin-tech/1727288/vasp-licencing-a-tool-for-advancing-the-future-of-virtual-assets-in-ghana)
- GS18: [Digital Policy Alert, CSA CII directive](https://digitalpolicyalert.org/event/24785-implemented-csa-directive-for-the-protection-of-critical-information-infrastructure-cii-highlighting-baseline-cybersecurity-measures-24-hour-incident-reporting-and-72-hour-vulnerability-disclosure)
- GS19: [regulations.ai summary of Act 1038](https://regulations.ai/regulations/RAI-GH-NA-ACT1038-2020)
- GS20: [Graphic on CSA licensing enforcement](https://www.graphic.com.gh/news/general-news/csa-to-enforce-strict-licensing-of-cybersecurity-providers-from-january-31.html)
- GS21: [Digital Policy Alert, Cybersecurity Amendment Bill](https://digitalpolicyalert.org/change/16824-licensing-requirements-in-cybersecurity-amendment-bill)
- GS22: [GNA on BoG and CSA alignment](https://gna.org.gh/?p=217831)
- GS23: [Graphic on BoG cyber watch and fraud losses](https://www.graphic.com.gh/business/business-news/mobile-money-and-bank-fraud-cost-ghana-ghc10m-in-2024-bog-to-tighten-cyber-watch.html)
- GS24: [Act 987 on GhaLII](https://ghalii.org/akn/gh/act/2019/987)
- GS25: [AllAfrica on CISD 2026](https://allafrica.com/stories/202603310611.html); [Ghanaian Times](https://ghanaiantimes.com.gh/bog-launches-new-cyber-security-directive-to-safeguard-financial-sector/); [Group-IB (vendor)](https://www.group-ib.com/resources/research-hub/cisd-2026-ghana-compliance/); [OpenSecurityArchitecture mapping of the 2018 CISD](https://opensecurityarchitecture.org/frameworks/bog-cisd)
- GS26: [Templars on data protection](https://www.templars-law.com/app/uploads/2023/05/Data-Protection-Compliance-in-Ghana_final.pdf); [Mondaq on data protection](https://www.mondaq.com/privacy-protection/1661208/data-protection-compliance-in-ghana-navigating-ghanas-data-protection-law); [B&FT on the 2025 Bill](https://thebftonline.com/2025/11/21/understanding-the-data-protection-bill-2025-key-highlights-for-businesses-and-individuals/)
- GS27: [Ghanaian Times on SIM re-registration](https://ghanaiantimes.com.gh/new-deadline-for-sim-re-registration-too-short-momo-agents-association/)
- GS28: [MyJoyOnline on the BoG 2025 fraud report](https://www.myjoyonline.com/financial-fraud-cases-in-ghana-rise-63-in-four-years-as-exposure-hits-gh%C2%A2101-million-bog/); [Citi Newsroom](https://www.citinewsroom.com/2026/07/bog-2025-fraud-report-the-numbers-beneath-the-headline/); [cash suppression](https://www.myjoyonline.com/bog-flags-gh%C2%A240-7m-cash-suppression-risk-after-single-gh%C2%A236-million-fraud-case/); [Cab-Beyuo on MoMo fraud](https://www.myjoyonline.com/the-fraudsters-dont-use-hacking-they-play-on-our-minds-bogs-cab-beyuo-on-mobile-money-fraud/)
- GS29: [Ghana Association of Banks fraud report, Sept to Dec 2025](https://gab.com.gh/assets/images/docs/FRAUD%20REPORT%20SEPT-DEC%202025.pdf)
- GS30: [FIC typology on personal accounts and tax evasion](https://fic.gov.gh/media/attachments/2026/01/22/suspicious-use-of-personal-bank-accounts-for-business-transactions-to-evade-payment-of-tax.pdf)
- GS31: [Mining Index on the Swissaid report](https://www.miningindex.co.zw/?p=109878); [Citi Newsroom on gold sector reforms](https://www.citinewsroom.com/2025/08/government-steps-up-gold-sector-reforms-ahead-of-anti-money-laundering-review/)
- GS32: [Graphic on 20 blacklisted loan apps](https://graphiconline.com/business/business-news/sika-tap-cedigo-popocedi-named-full-list-of-20-illegal-loan-apps-blacklisted-by-bank-of-ghana.html)
- GS33: [SEC investment scam alert](https://sec.gov.gh/investment-scam-alert-rise-of-fraudulent-investment-schemes-in-ghana/)
- GS34: [Graphic on 2025 mobile money volumes](https://www.graphic.com.gh/business/business-news/mobile-money-transactions-reach-ghc518b-in-2025.html)
- GS35: [MyJoyOnline on INTERPOL and MoMo](https://www.myjoyonline.com/ghanaians-lose-1-3m-to-mobile-money-fraud-in-first-quarter-of-2025-interpol-report/); [The Fourth Estate](https://thefourthestategh.com/2025/10/mobile-money-robberies-inside-ghanas-rising-cybercrime-wave/)
- GS36: [Citi Newsroom on EOCO romance scams](https://www.citinewsroom.com/2025/03/eoco-raises-alarm-over-rising-romance-scams/); [US DOJ, N.D. Ohio](https://www.justice.gov/usao-ndoh/pr/ghanaian-nationals-indicted-romance-fraud-scams)
- GS37: [GNA on E-Levy repeal](https://gna.org.gh/2025/03/parliament-abolishes-e-levy-returns-gh2-billion-to-ghanaians/)
- GS38: [FIC advisory 3 (25 August 2025)](https://fic.gov.gh/images/2026/advisory-notice---3.pdf); [FIC advisory 1 (30 May 2025)](https://fic.gov.gh/images/2026/Advisory_Notice_-_1.pdf)
- GS39: [FIC on the GIABA evaluation](https://fic.gov.gh/event-ghana-third-round-mutual-evaluation.html); [FIC on grey-list exit](https://fic.gov.gh/press-ghana-exits-fatf-grey-list.html)
- GS42: [BoG licence categories](https://www.bog.gov.gh/?p=32589); [Digital Policy Alert on the 2015 e-money notice](https://digitalpolicyalert.org/change/13133-bank-of-ghana-notice-no-bggovsec201509-to-e-money-issuers-and-the-general-public-regarding-the-guidelines-for-e-money-issuers-in-ghana-and-agent-guidelines)

**Rwanda**

- RS1: [RegAlert AML Rwanda listing](https://regalert.today/topics/aml/RW)
- RS2: [ESAAMLG Rwanda follow-up report, August 2025](https://www.esaamlg.org/reports/Rwanda%20FUR-August%202025.pdf)
- RS3: [RegAlert text of Law 001/2025](https://regalert.today/document/195f62e1-4ddf-4323-86ae-c8a19628da31/text)
- RS4: [RegAlert text of Law 028/2023](https://regalert.today/document/9e1e48ef-6e65-443d-859f-121cb0175664/text)
- RS5: [Regulation 002/FIC/2021 on Laws.Africa](https://commons.laws.africa/akn/rw/act/reg/2021/2/media/publication/rw-act-reg-2021-2-publication-document.pdf)
- RS6: [FATF second-round MER, July 2024](https://www.fatf-gafi.org/content/dam/fatf-gafi/fsrb-fur/Second%20Round%20MER%20of%20Rwanda-July%202024.pdf.coredownload.inline.pdf); [FATF MER page](https://www.fatf-gafi.org/content/fatf-gafi/en/publications/Mutualevaluations/MER-Rwanda-2024.html)
- RS7: [ENSafrica on the FIC amendment](https://www.ensafrica.com/news/detail/10825/strengthening-rwandas-anti-financial-crime-de)
- RS8: [Mondaq snapshot of the 2019 laws](https://www.mondaq.com/government-public-sector/904928/a-snapshot-of-rwandas-tightened-financial-crimes-laws)
- RS9: [RegAlert BNR Directive 02/2009](https://regalert.today/document/ac08f2bf-f6ed-4c9c-b67e-fd5ba81b3992/text)
- RS10: [RegAlert payments listing](https://regalert.today/topics/payments/RW); [Digital Policy Alert on Regulation 54/2022](https://digitalpolicyalert.org/event/26398-regulation-no-542022-governing-electronic-money-issuers-including-licensing-requirements-was-signed-by-the-governor-of-the-national-bank-of-rwanda)
- RS11: [RegAlert Regulation 54/2022](https://regalert.today/document/regulation-no-54-2022-governing-electronic-money-issuers-ad07272a-4fa7-49b6-b9d1-5c2b5ed3b147/text); [BNR PDF](https://www.bnr.rw/documents/Reg_governing_Emoney_issuers_2022.pdf)
- RS12: [Streamline Feed on eKash](https://streamlinefeed.co.ke/news/who-profits-from-rwanda-rwf-20-ekash-transaction-cap); [RBA on eKash](https://rba.rw/a-fully-connected-rwanda-what-ekash-interoperability-means-for-the-future-of-financial-inclusion/)
- RS13: [Andersen on Law 023/2026](https://rw.andersen.com/dist/virtual-assets.html); [PwC Rwanda](https://www.pwc.com/rw/en/assets/pdf/rwandas-new-virtual-law.pdf)
- RS14: [DLA Piper data protection](https://dlapiperdataprotection.com/index.html?c=RW&t=law); [Hogan Lovells](https://hlc.com/en/publications/key-features-of-the-new-rwandan-data-protection-law); [ConsentStack](https://www.consentstack.io/regulations/rw-law058)
- RS15: [Cyber Crimes Law PDF](https://e-ihuriro.rcsprwanda.org/wp-content/uploads/2023/05/Cyber_Crimes_Law.pdf)
- RS16: [National Cybersecurity Strategy 2024-2029](https://digitalpolicyalert.org/event/26134-rwanda-released-the-national-cybersecurity-strategy-2024-2029); [NCSA minimum standards](https://digitalpolicyalert.org/event/24887-adopted-ncsa-minimum-cybersecurity-standards-for-essential-service-providers-including-baseline-cybersecurity-requirements)
- RS17: [RegAlert Regulation 50/2022](https://regalert.today/document/7eef4428-1ace-4cf6-828e-ca9f2c3f8cea/text); [Digital Policy Alert](https://digitalpolicyalert.org/event/24884-adopted-national-bank-of-rwanda-regulation-on-cyber-security-in-regulated-institutions-no-502022-including-cyber-incidents-report-requirement)
- RS18: [RegAlert Directive 4230/2024-00037](https://regalert.today/document/0bc27f1a-18bd-4270-ae33-dd3c0d127976/text)
- RS19: [Digital Policy Alert on Law 061/2021](https://digitalpolicyalert.org/event/26394-law-no-0612021-governing-the-payment-system-including-licensing-requirements-enters-into-force); [RegAlert Regulation 74/2023](https://regalert.today/document/8b1091a0-483f-4d9b-95cd-877b7e144839/text)
- RS20: [RURA Telecom Network Security Regulations](https://rura.rw/fileadmin/Documents/docs/Regulations_Governing__Telecom_Network_Security.pdf)
- RS21: [RURA draft SIM registration regulation](https://www.rura.rw/fileadmin/Documents/ICT/Laws/Draft_Regulation_governing_SIM_cards_registration_for_January_2023.pdf); [Developing Telecoms on SIM registration](https://developingtelecoms.com/telecom-business/telecom-regulation/11841-rwanda-tightens-sim-registration-to-curb-fraud.html)
- RS22: [NCSA incident reporting](https://www.cyber.gov.rw/report-incident/)
- RS23: [New Times on digital fraud](https://www.newtimes.co.rw/article/36384/news/technology/digital-fraud-in-rwanda-declines-below-global-average/amp); [AllAfrica, February 2026](https://allafrica.com/stories/202602200031.html); [New Times, TransUnion survey](https://www.newtimes.co.rw/article/37302/news/technology/digital-fraud-surge-threatens-trust-as-scammers-target-six-in-10-consumers)
- RS24: [ITWeb Africa on Equity Bank Rwanda](https://itweb.africa/article/cyber-fraud-storm-hits-equity-bank-rwanda-as-35-arrested/lwrKx73YR81qmg1o)
- RS25: [KT Press on criminal proceeds](https://www.ktpress.rw/2025/07/how-rwf-93-billion-in-criminal-proceeds-flowed-through-rwandas-economy/); [2024 National Risk Assessment](https://icparwanda.com/wp-content/uploads/2025/05/2024-NRA-Report.pdf); [Zigram summary](https://www.zigram.tech/?p=33516)
- RS26: [Tranglo remittance limits (partner limits, not regulatory)](https://www.tranglo.com/?p=9223)
- RS27: [Andersen on foreign exchange regulation](https://rw.andersen.com/dist/foreign-exchange-regulation.html)
- RS28: [New Times on a 2017 SIM-box case](https://www.newtimes.co.rw/article/146833/News/three-arrested-over-rwf1bn-sim-box-fraud-targeting-mtn-rwanda/amp)
- RS29: [Crowdfund Insider on BNR pyramid warning, 2019](https://www.crowdfundinsider.com/2019/06/147945-rwandan-central-bank-issues-warning-against-pyramid-schemes-network-marketing-programs-and-scam-coins/)
- RS30: [Mondaq on PSP recategorisation](https://mondaq.com/corporate-governance/1506808/rwandas-payment-service-providers-must-recategorise-licenses-by-18-september-2024-or-risk-license-revocation)
- RS32: [RegAlert KYC listing](https://regalert.today/topics/kyc/RW)

**Cross-references:** the Nigeria library (`PROMPT_3_NIGERIA_RULES_AND_COMPLIANCE_DASHBOARD.md`) and the crypto report (`reports/Crypto compliance on and off chain.md`).
