// A time zone with clock changes, so tests of local-time date arithmetic fail
// on a UTC machine too. Workers start after this runs and inherit it.
export default function globalSetup(): void {
  process.env.TZ = 'Europe/Berlin';
}
