---
layout: page
title: Privacy &amp; Cookies
eyebrow: How this site handles your data
lead: This is the marketing and documentation site for Helena, the open-source API client. It is a static site with no accounts, no logins, and no advertising. The only thing that touches your data is opt-in, anonymous usage analytics — explained below.
description: Helena website privacy and cookie notice — opt-in, anonymous Google Analytics only, loaded solely after you Accept.
---

## Who is responsible (data controller)

The data controller for this site is **IDCT Bartosz Pachołek** (idct.tech):

> **IDCT Bartosz Pachołek**
> Kaszubska 12/8C
> 70-403 Szczecin, Poland
> NIP (VAT EU): PL7642542255

Contact: the [contact page]({{ '/contact/' | relative_url }}).

## What we collect

### The Helena desktop app — nothing

The Helena application collects **no data at all**. It has no accounts, no
telemetry, no analytics, and no runtime update check, and it never phones home.
The only network traffic it makes is the API requests **you** explicitly send;
your collections, environments, and secrets stay on your own machine. There is
nothing to opt out of because nothing is collected. The rest of this notice is
about **this website only**.

### This website

Only if you press **Accept** in the cookie banner do we load **Google Analytics 4**
to measure aggregate, anonymous usage — pages viewed, rough geography,
device/browser type, referring links, and a few anonymous interaction events
(copying a command, opening the GitHub repository, downloading a build). We use this
only to understand what's useful and improve the site. We do **not**:

- serve ads or use advertising cookies;
- track you across other websites;
- sell or share your data with third parties for marketing;
- attempt to identify you personally.

## When analytics loads

We use a **prior-consent** model. Google Analytics is **not loaded at all** until you
choose **Accept** — so if you **Reject**, or ignore the banner, **nothing is sent to
Google** and no analytics cookies are set. The site works fully either way.

## Cookies &amp; storage

Nothing analytics-related is stored or sent until you **Accept**. If you do, Google
Analytics sets:

| Cookie | Purpose | Retention |
| --- | --- | --- |
| `_ga` | Distinguishes anonymous visitors | ~2 years |
| `_ga_28S8FGLZGC` | Maintains the analytics session state | ~2 years |

Independently of your choice, a small **`localStorage`** entry (`helena-consent`)
records your Accept/Reject decision and its date so the banner doesn't reappear every
visit. That entry is strictly functional (not analytics) and we re-ask for consent
after about **180 days**.

## Legal basis

Analytics is processed **only on the basis of your consent** (GDPR Art. 6(1)(a)),
which you can withdraw at any time — see *Your choices* below. Withdrawing consent
does not affect processing that already happened.

## International transfer

When you Accept, analytics data is processed by Google LLC in the United States.
Google is certified under the EU–U.S. Data Privacy Framework, which the transfer
relies on (with Google's Standard Contractual Clauses as a fallback safeguard). GA4
does not store IP addresses.

## Your choices

- **Accept** or **Reject** in the banner — Reject keeps analytics fully off.
- Change your mind anytime via **Cookie preferences** in the footer, or by clearing
  this site's data in your browser.
- Install Google's [Analytics opt-out add-on](https://tools.google.com/dlpage/gaoptout).

## Your rights

Under the GDPR you have the right to access, rectify, erase, restrict, or object to
processing of your data, to withdraw consent, and to lodge a complaint with your
data-protection supervisory authority (in Poland, the UODO). To exercise any of
these, use the [contact page]({{ '/contact/' | relative_url }}).

## More information

Analytics data is handled per [Google's Privacy Policy](https://policies.google.com/privacy)
and [data-processing terms](https://business.safety.google/adsprocessorterms/). Questions?
Use the [contact page]({{ '/contact/' | relative_url }}).
