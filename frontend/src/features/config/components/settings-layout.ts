export const settingsLayout = {
  accountPolicyFields:
    "grid content-start items-start gap-4 lg:grid-cols-[minmax(0,1.15fr)_minmax(0,1fr)]",
  connection:
    "grid h-full min-h-0 auto-rows-[100%] items-stretch gap-4 xl:grid-cols-[minmax(0,1.45fr)_minmax(0,0.75fr)]",
  accounts:
    "grid h-full min-h-0 auto-rows-[100%] items-stretch gap-4 xl:grid-cols-[minmax(0,1.6fr)_minmax(0,0.7fr)]",
  interface: "xl:grid-cols-[minmax(0,1.5fr)_minmax(0,1fr)]",
  connectionFields:
    "grid min-h-0 flex-1 content-start gap-4 overflow-y-auto overscroll-contain lg:grid-cols-[minmax(0,1fr)_minmax(0,0.4fr)]",
  notificationFields: "grid min-h-0 flex-1 content-start gap-5 overflow-y-auto overscroll-contain",
  notificationCredentials: "grid items-start gap-x-5 gap-y-4 sm:grid-cols-2",
  notificationDestination:
    "grid items-start gap-x-5 gap-y-4 sm:grid-cols-[minmax(10rem,0.7fr)_minmax(0,1.3fr)]",
  logFields: "grid min-h-0 flex-1 content-start gap-3 overflow-y-auto overscroll-contain",
  logControls: "grid gap-x-5 gap-y-3 sm:grid-cols-[minmax(0,0.8fr)_minmax(0,1.2fr)]",
} as const;
