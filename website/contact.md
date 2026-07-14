---
layout: page
hero_image: /assets/img/app-hero-fancy.png
hero_alt: The Helena app showing a request and its JSON response
title: Contact
eyebrow: Say hello
lead: Questions, ideas, bug reports, or just want to chat about Helena? Send a message.
description: Get in touch about Helena - questions, ideas, and bug reports.
---

<div class="contact-grid">
  <div class="reveal">
    <h2 style="margin-top:0">Other ways to reach me</h2>
    <ul>
      <li><b>Chat:</b> the <a href="{{ site.discord }}">Helena Discord</a> for quick questions and ideas.</li>
      <li><b>Bugs &amp; features:</b> the <a href="{{ site.repo }}/issues">GitHub issue tracker</a> is the fastest path.</li>
      <li><b>Code:</b> <a href="{{ site.repo }}">{{ site.repo | remove: "https://" }}</a></li>
      <li><b>Web:</b> <a href="{{ site.author_url }}">idct.tech</a></li>
    </ul>
    <p class="muted">The form goes straight to my inbox. I read everything and try to reply within a few days.</p>

    <a class="discord-cta" href="{{ site.discord }}">{% include icon-discord.svg %} Join the Discord</a>
    <div class="qr-card">
      <img src="{{ '/assets/img/discord-qr.png' | relative_url }}" alt="QR code for the Helena Discord invite" width="264" height="264" loading="lazy" decoding="async">
      <span>Scan to join the Discord</span>
    </div>
  </div>

  <form class="contact-form reveal" action="https://api.web3forms.com/submit" method="POST">
    <input type="hidden" name="access_key" value="d777bd78-135d-4c53-82fd-d3ace8453ba0">
    <input type="hidden" name="subject" value="New message from the Helena website">
    <input type="hidden" name="from_name" value="Helena website">
    <input type="hidden" name="redirect" value="{{ '/contact/thank-you/' | absolute_url }}">
    <input type="checkbox" name="botcheck" hidden tabindex="-1" autocomplete="off">

    <div class="field">
      <label for="name">Name</label>
      <input type="text" name="name" id="name" placeholder="Your name" required>
    </div>

    <div class="field">
      <label for="email">Email</label>
      <input type="email" name="email" id="email" placeholder="you@example.com" required>
    </div>

    <div class="field">
      <label for="message">Message</label>
      <textarea name="message" id="message" rows="5" placeholder="What's on your mind?" required></textarea>
    </div>

    <div class="h-captcha" data-sitekey="50b2fe65-b00b-4b9e-ad62-3ba471098be2"></div>

    <button type="submit" class="btn btn-primary btn-block">Send message</button>
  </form>
</div>

<!-- Standard (non-JS) POST so Web3Forms honours the `redirect` field above and
     sends the visitor to /contact/thank-you/. hCaptcha renders via its own API
     with the Web3Forms shared site key (the AJAX client script would ignore the
     redirect). -->
<script src="https://js.hcaptcha.com/1/api.js" async defer></script>
