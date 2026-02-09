import test from "node:test";
import assert from "node:assert/strict";
import path from "node:path";
import os from "node:os";
import fs from "node:fs/promises";

type Disposable = { dispose(): void };
type CommandHandler = (...args: unknown[]) => unknown;
type ModuleLoad = (request: string, parent: NodeModule | null, isMain: boolean) => unknown;

test("generate diagnostic bundle command emits a redacted support artifact", async () => {
  const workspaceRoot = await fs.mkdtemp(path.join(os.tmpdir(), "dyalog-dap-ext-"));
  const moduleLoader = require("node:module") as { _load: ModuleLoad };
  const originalLoad = moduleLoader._load;
  const commandHandlers = new Map<string, CommandHandler>();
  const infoMessages: string[] = [];
  const previousTranscriptDir = process.env.DYALOG_RIDE_TRANSCRIPTS_DIR;
  const transcriptDir = path.join(workspaceRoot, "logs", "transcripts");
  process.env.DYALOG_RIDE_TRANSCRIPTS_DIR = transcriptDir;

  const launchConfigurations = [
    {
      name: "Dyalog: Launch (RIDE)",
      type: "dyalog-dap",
      request: "launch",
      rideAddr: "127.0.0.1:4502",
      rideTranscriptsDir: "${workspaceFolder}/custom-transcripts",
      adapterEnv: {
        API_TOKEN: "secret-value"
      }
    }
  ];

  const vscodeStub = {
    window: {
      createOutputChannel() {
        return {
          appendLine(): void {},
          show(): void {},
          dispose(): void {}
        };
      },
      showInformationMessage(message: string): Promise<string> {
        infoMessages.push(message);
        return Promise.resolve(message);
      },
      showErrorMessage(message: string): Promise<string> {
        return Promise.resolve(message);
      },
      showInputBox(): Promise<undefined> {
        return Promise.resolve(undefined);
      }
    },
    commands: {
      registerCommand(command: string, handler: CommandHandler): Disposable {
        commandHandlers.set(command, handler);
        return {
          dispose(): void {
            commandHandlers.delete(command);
          }
        };
      }
    },
    debug: {
      activeDebugSession: {
        configuration: launchConfigurations[0]
      },
      registerDebugConfigurationProvider(): Disposable {
        return { dispose(): void {} };
      },
      registerDebugAdapterDescriptorFactory(): Disposable {
        return { dispose(): void {} };
      }
    },
    workspace: {
      workspaceFolders: [
        {
          name: "integration-workspace",
          uri: {
            fsPath: workspaceRoot
          }
        }
      ],
      fs: {
        async createDirectory(uri: { fsPath: string }): Promise<void> {
          await fs.mkdir(uri.fsPath, { recursive: true });
        },
        async writeFile(uri: { fsPath: string }, content: Uint8Array): Promise<void> {
          await fs.mkdir(path.dirname(uri.fsPath), { recursive: true });
          await fs.writeFile(uri.fsPath, Buffer.from(content));
        }
      },
      getConfiguration(section: string): {
        get<T>(key: string, defaultValue: T): T;
        update(_key: string, _value: unknown): Promise<void>;
      } {
        if (section === "launch") {
          return {
            get<T>(key: string, defaultValue: T): T {
              if (key === "configurations") {
                return launchConfigurations as unknown as T;
              }
              return defaultValue;
            },
            update(): Promise<void> {
              return Promise.resolve();
            }
          };
        }
        if (section === "dyalogDap") {
          return {
            get<T>(key: string, defaultValue: T): T {
              if (key === "diagnostics.verbose") {
                return false as T;
              }
              return defaultValue;
            },
            update(): Promise<void> {
              return Promise.resolve();
            }
          };
        }
        return {
          get<T>(_key: string, defaultValue: T): T {
            return defaultValue;
          },
          update(): Promise<void> {
            return Promise.resolve();
          }
        };
      }
    },
    Uri: {
      joinPath(base: { fsPath: string }, ...segments: string[]): { fsPath: string } {
        return { fsPath: path.join(base.fsPath, ...segments) };
      }
    },
    ConfigurationTarget: {
      Global: 1
    }
  };

  let extensionPath = "";
  try {
    moduleLoader._load = ((request: string, parent: NodeModule | null, isMain: boolean): unknown => {
      if (request === "vscode") {
        return vscodeStub;
      }
      return originalLoad(request, parent, isMain);
    }) as ModuleLoad;

    extensionPath = require.resolve("../extension");
    delete require.cache[extensionPath];
    const extension = require("../extension") as {
      activate(context: { subscriptions: Disposable[]; extension: { packageJSON: { version: string } } }): void;
    };

    const context = {
      subscriptions: [] as Disposable[],
      extension: {
        packageJSON: {
          version: "0.0.1-test"
        }
      }
    };
    extension.activate(context);

    const command = commandHandlers.get("dyalogDap.generateDiagnosticBundle");
    assert.ok(command, "expected dyalogDap.generateDiagnosticBundle command registration");
    await Promise.resolve(command());

    const supportDir = path.join(workspaceRoot, ".dyalog-dap", "support");
    const entries = await fs.readdir(supportDir);
    assert.equal(entries.length, 1, "expected a single diagnostic bundle artifact");
    assert.match(entries[0], /^diagnostic-bundle-/);

    const bundlePath = path.join(supportDir, entries[0]);
    const bundle = JSON.parse(await fs.readFile(bundlePath, "utf8")) as Record<string, any>;
    assert.equal(bundle.extension.version, "0.0.1-test");
    assert.equal(bundle.workspace.name, "integration-workspace");
    assert.ok(Array.isArray(bundle.diagnostics.recent));
    assert.ok(
      (bundle.diagnostics.recent as string[]).some((line) => line.includes("extension.activate")),
      "expected extension diagnostics in bundle"
    );
    assert.equal(bundle.environment.DYALOG_RIDE_TRANSCRIPTS_DIR, "<redacted-path>");
    assert.equal(bundle.configSnapshot.launchConfigurations[0].adapterEnv.API_TOKEN, "<redacted>");
    assert.ok(
      (bundle.transcripts.pointers as string[]).includes(path.join(workspaceRoot, "custom-transcripts")),
      "expected launch rideTranscriptsDir pointer"
    );
    assert.ok(
      (bundle.transcripts.pointers as string[]).includes(path.join(workspaceRoot, "artifacts", "integration")),
      "expected default artifacts pointer"
    );
    assert.match(infoMessages.at(-1) ?? "", /diagnostic bundle written/i);
  } finally {
    if (extensionPath !== "") {
      delete require.cache[extensionPath];
    }
    moduleLoader._load = originalLoad;
    if (previousTranscriptDir === undefined) {
      delete process.env.DYALOG_RIDE_TRANSCRIPTS_DIR;
    } else {
      process.env.DYALOG_RIDE_TRANSCRIPTS_DIR = previousTranscriptDir;
    }
    await fs.rm(workspaceRoot, { recursive: true, force: true });
  }
});
