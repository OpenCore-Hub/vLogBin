/* vLogBin Prototype interactions */
(function () {
  // Environment switcher: test / live
  document.querySelectorAll(".env-switch").forEach(function (sw) {
    sw.addEventListener("click", function (e) {
      var btn = e.target.closest("button");
      if (!btn) return;
      sw.querySelectorAll("button").forEach(function (b) {
        b.classList.toggle("active", b === btn);
      });
      document.querySelectorAll(".env-marker").forEach(function (m) {
        m.textContent = btn.dataset.env || "test";
      });
    });
  });

  // Tabs
  document.querySelectorAll("[data-tabs]").forEach(function (wrap) {
    var btns = wrap.querySelectorAll("[data-tab]");
    btns.forEach(function (b) {
      b.addEventListener("click", function () {
        btns.forEach(function (x) {
          x.classList.toggle("active", x === b);
        });
        var target = b.dataset.tab;
        document.querySelectorAll("[data-pane]").forEach(function (p) {
          p.style.display = p.dataset.pane === target ? "" : "none";
        });
      });
    });
  });

  // Sidebar sub-group toggle
  document.querySelectorAll(".nav-item.has-sub").forEach(function (item) {
    item.addEventListener("click", function () {
      var sub = item.nextElementSibling;
      if (sub && sub.classList.contains("nav-sub")) {
        var open = sub.style.display !== "none";
        sub.style.display = open ? "none" : "block";
        var chev = item.querySelector(".chev");
        if (chev) chev.style.transform = open ? "" : "rotate(90deg)";
      }
    });
  });

  // Command palette hint (demo)
  var search = document.querySelector("[data-global-search]");
  if (search) {
    search.addEventListener("keydown", function (e) {
      if (e.key === "/" && e.target === document.body) {
        e.preventDefault();
        search.focus();
      }
    });
  }
})();
