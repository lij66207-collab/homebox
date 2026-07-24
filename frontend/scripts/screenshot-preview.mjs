/* 临时预览截图：桌面 + 移动视口（用后即删） */
import { chromium } from "@playwright/test";

const base = process.env.BASE_URL || "http://localhost:3000";
const email = process.env.SHOT_EMAIL || "";
const password = process.env.SHOT_PASSWORD || "";
const tag = process.env.SHOT_TAG || "preview";

const browser = await chromium.launch();

async function shoot(name, viewport, mobile = false) {
  const page = await browser.newPage({ viewport, isMobile: mobile, hasTouch: mobile });
  await page.goto(base + "/", { waitUntil: "domcontentloaded" });
  await page.waitForTimeout(6000);
  await page.screenshot({ path: `/tmp/${tag}-${name}-login.png` });

  if (email && password) {
    await page.fill("#login-username", email);
    await page.fill("#login-password", password);
    await page.click('button[type="submit"]');
    await page.waitForTimeout(4000);
    await page.waitForLoadState("networkidle").catch(() => {});
    await page.screenshot({ path: `/tmp/${tag}-${name}-home.png` });

    await page.goto(base + "/items", { waitUntil: "domcontentloaded" });
    await page.waitForTimeout(4000);
    await page.screenshot({ path: `/tmp/${tag}-${name}-items.png` });
  }
  await page.close();
  console.log(`${name} done`);
}

await shoot("desktop", { width: 1280, height: 800 });
await shoot("mobile", { width: 390, height: 844 }, true);

await browser.close();
