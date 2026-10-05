import { defineConfig } from 'vitest/config'

// Unit tests for pure frontend logic (search, sorting, ...). They run in Node:
// modules that touch the Wails runtime (data.ts, components) must only be
// imported as types from tests.
export default defineConfig({
  test: {
    environment: 'node',
    include: ['src/**/*.test.{ts,tsx}'],
  },
})
