# Official SDK fixture

`official-sdk-20260219f9f6.js` is the unmodified public SDK retrieved from
`https://sentinel.openai.com/sentinel/20260219f9f6/sdk.js` on 2026-09-20.
It contains no account data, cookies, tokens or captured user traffic.
It is used only to exercise the Go compatibility boundary against real SDK code.
Production downloads the currently advertised SDK from the fixed official host;
it does not substitute this test fixture.

The tests replace only the HTTP response for the SDK requirements endpoint.
Passing these tests does not establish compatibility with every future SDK,
challenge type, or real account login.
