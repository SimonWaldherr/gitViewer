/**
 * Small helper script for the gitViewer UI.
 *
 * @file
 */
(() => {
  "use strict";

  const THEME_KEY = "gitViewer.theme";

  /**
   * Apply a theme token to the document root.
   *
   * @param {"light"|"dark"} theme
   *   The theme to apply.
   */
  function applyTheme(theme) {
    document.documentElement.dataset.theme = theme;
  }

  /**
   * Update the text content of the theme toggle button.
   *
   * @param {HTMLButtonElement} button
   *   The toggle button.
   * @param {"light"|"dark"} theme
   *   The currently active theme.
   */
  function updateButtonLabel(button, theme) {
    button.textContent = theme === "dark" ? "Light mode" : "Dark mode";
  }

  /**
   * Initialize the light/dark theme toggle.
   *
  * Reads and writes `gitViewer.theme` from localStorage.
   */
  function initThemeToggle() {
    /** @type {HTMLButtonElement|null} */
    const button = document.querySelector("[data-role='theme-toggle']");
    if (!button) return;

    /** @type {"light"|"dark"} */
    let theme = /** @type {"light"|"dark"} */ (
      localStorage.getItem(THEME_KEY) || "dark"
    );

    applyTheme(theme);
    updateButtonLabel(button, theme);

    button.addEventListener("click", () => {
      theme = theme === "dark" ? "light" : "dark";
      applyTheme(theme);
      localStorage.setItem(THEME_KEY, theme);
      updateButtonLabel(button, theme);
    });
  }

  /**
   * Initialize simple "collapse" toggles.
   *
   * Any element with `[data-toggle='collapse']` will toggle the
   * `hidden` attribute on the element whose id is given by
   * `data-target`.
   */
  function initCollapsibles() {
    /** @type {NodeListOf<HTMLElement>} */
    const triggers = document.querySelectorAll("[data-toggle='collapse']");
    triggers.forEach(trigger => {
      const targetId = trigger.getAttribute("data-target");
      if (!targetId) return;
      const target = document.getElementById(targetId);
      if (!target) return;

      trigger.addEventListener("click", () => {
        const isHidden = target.hasAttribute("hidden");
        if (isHidden) {
          target.removeAttribute("hidden");
        } else {
          target.setAttribute("hidden", "hidden");
        }
      });
    });
  }

  document.addEventListener("DOMContentLoaded", () => {
    initThemeToggle();
    initCollapsibles();
  });
})();
