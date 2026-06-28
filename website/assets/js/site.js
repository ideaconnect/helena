/* Helena site - tiny vanilla enhancements (no dependencies). Progressive:
   if anything here fails, content is still fully visible and usable. */
(function () {
  "use strict";

  // 1) Reveal-on-scroll. Elements with .reveal start hidden (only while the
  //    `js` class is on <html>); we add .in as they enter the viewport.
  var revealables = [].slice.call(document.querySelectorAll(".reveal"));
  function revealAll() { revealables.forEach(function (el) { el.classList.add("in"); }); }

  if (!("IntersectionObserver" in window) || revealables.length === 0) {
    revealAll();
  } else {
    var io = new IntersectionObserver(function (entries) {
      entries.forEach(function (e) {
        if (e.isIntersecting) { e.target.classList.add("in"); io.unobserve(e.target); }
      });
    }, { rootMargin: "0px 0px -8% 0px", threshold: 0.08 });
    revealables.forEach(function (el) { io.observe(el); });
    // Safety net: never leave anything hidden once the page has fully loaded.
    window.addEventListener("load", function () {
      setTimeout(revealAll, 1200);
    });
  }

  // 2) Stronger nav shadow once the page is scrolled.
  var nav = document.getElementById("nav");
  if (nav) {
    var onScroll = function () { nav.classList.toggle("scrolled", window.scrollY > 8); };
    onScroll();
    window.addEventListener("scroll", onScroll, { passive: true });
  }
})();
