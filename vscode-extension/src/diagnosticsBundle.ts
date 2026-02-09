type Primitive = string | number | boolean | null;

export type DiagnosticBundleInput = {
  extensionVersion: string;
  workspaceName: string;
  diagnostics: string[];
  env: Record<string, string | undefined>;
  configSnapshot: unknown;
  transcriptPointers: string[];
};

export type DiagnosticBundle = {
  schemaVersion: string;
  generatedAt: string;
  extension: {
    name: string;
    version: string;
  };
  workspace: {
    name: string;
  };
  diagnostics: {
    recent: string[];
  };
  environment: Record<string, unknown>;
  configSnapshot: unknown;
  transcripts: {
    pointers: string[];
  };
};

const sensitiveKeyPattern =
  /(token|secret|password|passwd|api[-_]?key|authorization|cookie|credential|private[-_]?key|access[-_]?key|(?:^|[_-])pat(?:$|[_-])|pat$)/i;
const endpointKeyPattern = /(addr|endpoint|host)/i;
const pathKeyPattern = /(path|bin|launch|cwd|dir)/i;

export function buildDiagnosticBundle(input: DiagnosticBundleInput): DiagnosticBundle {
  const diagnostics = input.diagnostics.slice(-500);
  return {
    schemaVersion: "1",
    generatedAt: new Date().toISOString(),
    extension: {
      name: "dyalog-dap",
      version: input.extensionVersion
    },
    workspace: {
      name: input.workspaceName
    },
    diagnostics: {
      recent: diagnostics
    },
    environment: redactBundleValue(input.env) as Record<string, unknown>,
    configSnapshot: redactBundleValue(input.configSnapshot),
    transcripts: {
      pointers: input.transcriptPointers
    }
  };
}

export function redactBundleValue(value: unknown, key = ""): unknown {
  const keyText = key.toLowerCase();
  if (sensitiveKeyPattern.test(keyText)) {
    return "<redacted>";
  }

  if (Array.isArray(value)) {
    return value.map((item) => redactBundleValue(item, key));
  }

  if (isRecord(value)) {
    const out: Record<string, unknown> = {};
    for (const [childKey, childValue] of Object.entries(value)) {
      out[childKey] = redactBundleValue(childValue, childKey);
    }
    return out;
  }

  if (isPrimitive(value)) {
    if (typeof value === "string") {
      if (endpointKeyPattern.test(keyText)) {
        return "<redacted-endpoint>";
      }
      if (pathKeyPattern.test(keyText)) {
        return "<redacted-path>";
      }
    }
    return value;
  }

  return String(value);
}

function isPrimitive(value: unknown): value is Primitive {
  return (
    value === null ||
    typeof value === "string" ||
    typeof value === "number" ||
    typeof value === "boolean"
  );
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === "object" && !Array.isArray(value);
}
