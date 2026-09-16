import "@testing-library/jest-dom/vitest"

if (!Element.prototype.hasPointerCapture) {
  Element.prototype.hasPointerCapture = () => false
}