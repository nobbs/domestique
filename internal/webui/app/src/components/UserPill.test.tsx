import { describe, expect, it } from "vitest";
import { initialsOf } from "./UserPill";

describe("initialsOf", () => {
  it("takes the local part, because every address shares its domain", () => {
    expect(initialsOf("rider@example.test")).toBe("R");
  });

  // A separator in the local part usually parts a given name from a family
  // one, and two initials tell two addresses apart where the first two letters
  // of one name would not.
  it("reads a separated local part as two names", () => {
    expect(initialsOf("alexej.disterhoft@example.test")).toBe("AD");
    expect(initialsOf("jean-luc@example.test")).toBe("JL");
    expect(initialsOf("a_b@example.test")).toBe("AB");
    expect(initialsOf("rider+wahoo@example.test")).toBe("RW");
  });

  // The provider hands over a name when the account has one and no email, so
  // the display value is not always an address.
  it("reads a plain name as two names", () => {
    expect(initialsOf("Demo Rider")).toBe("DR");
    expect(initialsOf("Rider")).toBe("R");
  });

  it("takes only the first two, however many parts there are", () => {
    expect(initialsOf("one.two.three.four@example.test")).toBe("OT");
  });

  it("does not mistake a repeated separator for a name", () => {
    expect(initialsOf("first..last@example.test")).toBe("FL");
  });

  /*
   * The gate will not admit an address the service was not configured with, so
   * none of these can arrive from it. They are here because the value crosses
   * the wire, and a corner of the bar is a poor place to find that out.
   */
  it("says nothing rather than throwing on an address that is not one", () => {
    expect(initialsOf("")).toBe("");
    expect(initialsOf("@example.test")).toBe("");
    expect(initialsOf("  rider@example.test  ")).toBe("R");
  });
});
