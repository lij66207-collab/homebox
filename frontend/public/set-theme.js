try {
  const prefs = JSON.parse(localStorage.getItem("homebox/preferences/location") || "{}");
  let mode = prefs.theme;
  if (mode !== "light" && mode !== "dark" && mode !== "system") {
    mode = "system";
  }
  const dark = mode === "dark" || (mode === "system" && window.matchMedia("(prefers-color-scheme: dark)").matches);
  document.documentElement.classList.toggle("dark", dark);
  document.documentElement.setAttribute("data-theme", dark ? "dark" : "light");
} catch (e) {
  console.error("Failed to set theme", e);
}
