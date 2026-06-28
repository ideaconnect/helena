---
layout: page
title: Contact
eyebrow: Say hello
lead: Questions, ideas, bug reports, or just want to chat about Helena? Send a message.
description: Get in touch about Helena - questions, ideas, and bug reports.
---

<div class="contact-grid">
  <div class="reveal">
    <h2 style="margin-top:0">Other ways to reach me</h2>
    <ul>
      <li><b>Bugs &amp; features:</b> the <a href="{{ site.repo }}/issues">GitHub issue tracker</a> is the fastest path.</li>
      <li><b>Code:</b> <a href="{{ site.repo }}">{{ site.repo | remove: "https://" }}</a></li>
      <li><b>Web:</b> <a href="{{ site.author_url }}">idct.tech</a></li>
    </ul>
    <p style="color:var(--muted)">The form goes straight to my inbox. I read everything and try to reply within a few days.</p>
  </div>

  <form class="contact-form reveal" action="https://api.web3forms.com/submit" method="POST">
    <input type="hidden" name="access_key" value="3bb680f8-d4ed-4eb6-a7e5-f3650c726b8f">
    <input type="hidden" name="subject" value="New message from the Helena website">
    <input type="hidden" name="from_name" value="Helena website">
    <input type="checkbox" name="botcheck" style="display:none" tabindex="-1" autocomplete="off">

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

    <div class="h-captcha" data-captcha="true"></div>

    <button type="submit" class="btn btn-primary" style="width:100%">Send message</button>
    <div id="result" class="form-note"></div>
  </form>
</div>

<script src="https://web3forms.com/client/script.js" async defer></script>
