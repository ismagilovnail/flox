import { describe, expect, it } from "vitest";

import {
  constraints,
  curlExample,
  expandable,
  filterGroups,
  groupOperations,
  sampleValue,
  successResponse,
  typeLabel,
  type OpenApiDocument,
} from "@/lib/openapi";

const doc: OpenApiDocument = {
  openapi: "3.1.0",
  info: { title: "FLOX API", version: "1.0" },
  tags: [{ name: "Campaigns" }, { name: "Reports" }, { name: "Empty" }],
  paths: {
    "/campaigns/{id}": {
      parameters: [{ $ref: "#/components/parameters/Id" }],
      get: {
        operationId: "getCampaign",
        summary: "Get a campaign",
        tags: ["Campaigns"],
        "x-flox-scope": "read",
        responses: {
          "200": { description: "The campaign.", content: { "application/json": { schema: { $ref: "#/components/schemas/Campaign" } } } },
          default: { $ref: "#/components/responses/Error" },
        },
      },
      delete: {
        operationId: "deleteCampaign",
        summary: "Delete a campaign",
        tags: ["Campaigns"],
        "x-flox-scope": "write",
        responses: { "204": { description: "Deleted." } },
      },
    },
    "/analytics/report": {
      post: {
        operationId: "report",
        summary: "Report",
        tags: ["Reports"],
        "x-flox-scope": "read",
        requestBody: {
          content: {
            "application/json": {
              schema: {
                type: "object",
                required: ["from", "to", "dims"],
                properties: {
                  from: { type: "string", format: "date" },
                  to: { type: "string", format: "date" },
                  dims: { type: "array", items: { $ref: "#/components/schemas/Dimension" } },
                  timezone: { type: "string", example: "Europe/Berlin" },
                },
              },
            },
          },
        },
        responses: { "200": { description: "Rows." } },
      },
    },
  },
  components: {
    parameters: { Id: { name: "id", in: "path", required: true, schema: { type: "string" } } },
    schemas: {
      Campaign: { type: "object", properties: { id: { type: "string" } } },
      Dimension: { type: "string", enum: ["campaign", "country"] },
    },
  },
};

describe("groupOperations", () => {
  it("groups by tag in the document's order, resolves path parameters, drops empty tags", () => {
    const groups = groupOperations(doc);
    expect(groups.map((g) => g.name)).toEqual(["Campaigns", "Reports"]);
    const [get, del] = groups[0].operations;
    expect([get.method, del.method]).toEqual(["GET", "DELETE"]);
    expect(get.parameters).toEqual([{ name: "id", in: "path", required: true, schema: { type: "string" } }]);
  });

  it("filters by path, summary, id or tag", () => {
    const groups = groupOperations(doc);
    expect(filterGroups(groups, "delete").flatMap((g) => g.operations.map((o) => o.operation.operationId))).toEqual(["deleteCampaign"]);
    expect(filterGroups(groups, "/analytics").map((g) => g.name)).toEqual(["Reports"]);
    expect(filterGroups(groups, "reports").map((g) => g.name)).toEqual(["Reports"]);
    expect(filterGroups(groups, "nothing like this")).toEqual([]);
    expect(filterGroups(groups, "  ")).toBe(groups);
  });

  it("finds the one success response, a 204 without a schema", () => {
    const [get, del] = groupOperations(doc)[0].operations;
    expect(successResponse(get)).toEqual({ status: "200", description: "The campaign.", schema: { $ref: "#/components/schemas/Campaign" } });
    expect(successResponse(del)).toEqual({ status: "204", description: "Deleted.", schema: undefined });
  });
});

describe("typeLabel", () => {
  it.each([
    [{ $ref: "#/components/schemas/Campaign" }, "Campaign"],
    [{ type: "string", format: "date-time" }, "string · date-time"],
    [{ type: ["integer", "null"] }, "integer | null"],
    [{ type: "array", items: { $ref: "#/components/schemas/Offer" } }, "Offer[]"],
    [{ type: "object", additionalProperties: { type: "string" } }, "map of string"],
    [{ anyOf: [{ $ref: "#/components/schemas/Dimension" }, { type: "string", enum: ["day"] }] }, "Dimension | day"],
    [{}, "any"],
  ])("%j → %s", (schema, label) => {
    expect(typeLabel(schema)).toBe(label);
  });
});

describe("schema helpers", () => {
  it("expands objects, references and arrays of them only", () => {
    expect(expandable({ type: "string" })).toBeUndefined();
    expect(expandable({ type: "array", items: { type: "string" } })).toBeUndefined();
    expect(expandable({ type: "array", items: { $ref: "#/x" } })).toEqual({ $ref: "#/x" });
    expect(expandable({ anyOf: [{ $ref: "#/x" }, { type: "null" }] })).toEqual({ $ref: "#/x" });
  });

  it("states the rules", () => {
    expect(constraints({ minLength: 2, maxLength: 80 })).toEqual(["2–80 chars"]);
    expect(constraints({ minimum: 1, maximum: 500, default: 500 })).toEqual(["1–500", "default 500"]);
    expect(constraints({ exclusiveMinimum: 0, pattern: "^[A-Z]{3}$" })).toEqual(["> 0", "/^[A-Z]{3}$/"]);
  });

  it("builds the smallest valid body: required fields, examples, enum firsts", () => {
    const body = groupOperations(doc)[1].operations[0].operation.requestBody!.content!["application/json"].schema!;
    expect(sampleValue(doc, body)).toEqual({ from: "2026-10-01", to: "2026-10-01", dims: ["campaign"] });
    expect(sampleValue(doc, { type: "string", example: "Europe/Berlin" })).toBe("Europe/Berlin");
    expect(sampleValue(doc, { type: ["integer", "null"] })).toBe(1);
  });
});

describe("curlExample", () => {
  it("has the key header, placeholders for path parameters and a body when there is one", () => {
    const [get] = groupOperations(doc)[0].operations;
    expect(curlExample(doc, "https://api.flox.test/v1/", get)).toBe(
      'curl -X GET "https://api.flox.test/v1/campaigns/<id>" \\\n  -H "Authorization: Bearer $FLOX_API_KEY"',
    );
    const [report] = groupOperations(doc)[1].operations;
    expect(curlExample(doc, "https://api.flox.test/v1", report)).toContain(
      `-H "Content-Type: application/json" \\\n  -d '{\n    "from": "2026-10-01",\n    "to": "2026-10-01",\n    "dims": [\n      "campaign"\n    ]\n  }'`,
    );
  });
});
