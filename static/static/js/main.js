(() => {
  const header = document.querySelector(".site-header");
  const toggle = document.querySelector(".menu-toggle");
  const navigation = document.querySelector("#site-navigation");
  if (!header || !toggle || !navigation) return;

  const mobile = window.matchMedia("(max-width: 767px)");
  const links = [...navigation.querySelectorAll(".nav-links a")];
  const sections = links
    .map((link) => document.querySelector(link.hash))
    .filter(Boolean);

  const setMenu = (open, restoreFocus = false) => {
    toggle.setAttribute("aria-expanded", String(open));
    toggle.setAttribute("aria-label", open ? "关闭导航菜单" : "打开导航菜单");
    navigation.hidden = mobile.matches && !open;
    if (restoreFocus) toggle.focus();
  };
  const syncMenu = () => {
    toggle.hidden = !mobile.matches;
    setMenu(false);
  };
  toggle.addEventListener("click", () =>
    setMenu(toggle.getAttribute("aria-expanded") !== "true"),
  );
  mobile.addEventListener("change", syncMenu);
  syncMenu();

  document.addEventListener("keydown", (event) => {
    if (
      event.key === "Escape" &&
      toggle.getAttribute("aria-expanded") === "true"
    )
      setMenu(false, true);
  });
  document.addEventListener("click", (event) => {
    if (
      !header.contains(event.target) &&
      toggle.getAttribute("aria-expanded") === "true"
    )
      setMenu(false);
  });
  navigation.addEventListener("click", (event) => {
    const link = event.target.closest("a");
    if (!link || !mobile.matches) return;
    const wasOpen = toggle.getAttribute("aria-expanded") === "true";
    setMenu(false);
    if (
      wasOpen &&
      link.hash &&
      link.origin === location.origin &&
      link.pathname === location.pathname
    ) {
      const target = document.getElementById(link.hash.slice(1));
      if (target) {
        target.setAttribute("tabindex", "-1");
        target.focus({ preventScroll: true });
      }
    }
  });

  let scheduled = false;
  const updateScrollState = () => {
    header.classList.toggle("is-scrolled", window.scrollY > 16);
    const offset = header.getBoundingClientRect().height + 48;
    let current = "";
    for (const section of sections) {
      if (section.getBoundingClientRect().top <= offset)
        current = `#${section.id}`;
    }
    for (const link of links) {
      if (link.hash === current) link.setAttribute("aria-current", "location");
      else link.removeAttribute("aria-current");
    }
    scheduled = false;
  };
  window.addEventListener(
    "scroll",
    () => {
      if (!scheduled) {
        scheduled = true;
        window.requestAnimationFrame(updateScrollState);
      }
    },
    { passive: true },
  );
  updateScrollState();
})();
