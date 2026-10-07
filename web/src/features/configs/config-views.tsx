import { Button } from "../../components/Button";
import { FormEvent, useState } from "react";
import { Box, FileCode2, Network, Plus, Settings, Trash } from "lucide-react";
import { api, errorMessage, type Config, type ConfigVariable } from "../../shared/domain";
import { CodeEditor } from "../../shared/code-editor";
import { Empty, Field, FormActions, PageHeading } from "../../components";

/** List reusable game configurations and open the configuration editor. */
export function ConfigsPage({
  configs,
  onCreate,
  onEdit,
  canManage = false,
}: {
  configs: Config[];
  onCreate: () => void;
  onEdit: (t: Config) => void;
  canManage?: boolean;
}) {
  return (
    <div className="page-stack">
      <PageHeading
        eyebrow="GAME CATALOG"
        title="Configs"
        description="Build and maintain reusable configurations for your servers."
        action={
          canManage ? (
            <Button variant="primary" onClick={onCreate}>
              <Plus size={16} />
              Create config
            </Button>
          ) : undefined
        }
      />
      {configs.length ? (
        <div className="config-grid">
          {configs.map((t) => (
            <article className="config-card" key={t.id}>
              <div className="config-card-top">
                <div className="entity-icon large">
                  <FileCode2 size={18} />
                </div>
                <span className="config-slug">{t.slug}</span>
              </div>
              <h2>{t.name}</h2>
              <p>{t.description || "No description provided."}</p>
              <div className="config-image">
                <span>DOCKER IMAGE</span>
                <code>{t.spec?.dockerImages?.[0] || "Missing image"}</code>
              </div>
              <div className="config-meta">
                <span>
                  <Settings size={14} />
                  {(t.spec?.variables || []).length} variables
                </span>
                <span>
                  <Network size={14} />
                  {(t.spec?.ports || []).length} ports
                </span>
              </div>
              {canManage && (
                <Button
                  variant="subtle"
                  fullWidth

                  onClick={() => onEdit(t)}
                >
                  Edit config
                </Button>
              )}
            </article>
          ))}
        </div>
      ) : (
        <Empty
          icon={Box}
          title="No configs"
          text="Create a Docker config or restore your starter configs."
          action={
            canManage ? (
              <Button variant="primary" onClick={onCreate}>
                <Plus size={15} />
                Create config
              </Button>
            ) : undefined
          }
        />
      )}
    </div>
  );
}

/** Create or edit a configuration, including its startup and install settings. */
export function ConfigForm({
  config,
  availableConfigs,
  onSaved,
  onCancel,
}: {
  config: Config | null;
  availableConfigs: Config[];
  onSaved: () => void;
  onCancel: () => void;
}) {
  const [name, setName] = useState(config?.name || "");
  const [slug, setSlug] = useState(config?.slug || "");
  const [description, setDescription] = useState(config?.description || "");
  const [spec, setSpec] = useState(JSON.stringify(config?.spec || {}, null, 2));
  const [starterId, setStarterId] = useState("");
  const [editorMode, setEditorMode] = useState<"fields" | "json">("fields");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  let parsed: Record<string, unknown> | null = null;
  try {
    const value: unknown = JSON.parse(spec);
    if (value && typeof value === "object" && !Array.isArray(value)) {
      parsed = value as Record<string, unknown>;
    }
  } catch {
    /* JSON editor remains available */
  }
  const vars: ConfigVariable[] = Array.isArray(parsed?.variables)
    ? (parsed.variables as ConfigVariable[])
    : [];
  const updateVariables = (next: ConfigVariable[]) => {
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return;
    setSpec(JSON.stringify({ ...parsed, variables: next }, null, 2));
  };
  const updateVariable = (index: number, patch: Record<string, unknown>) =>
    updateVariables(
      vars.map((variable, i) =>
        i === index ? { ...variable, ...patch } : variable,
      ),
    );
  const changeName = (value: string) => {
    setName(value);
    if (!config) {
      setSlug(
        value
          .toLowerCase()
          .trim()
          .replace(/[^a-z0-9]+/g, "-")
          .replace(/^-|-$/g, ""),
      );
    }
  };
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      const payload = { name, slug, description, spec: JSON.parse(spec) };
      await api(config ? "/api/configs/" + config.id : "/api/configs", {
        method: config ? "PUT" : "POST",
        body: JSON.stringify(payload),
      });
      onSaved();
    } catch (err: unknown) {
      setError(
        err instanceof SyntaxError
          ? "Config specification must be valid JSON."
          : errorMessage(err),
      );
    } finally {
      setBusy(false);
    }
  };
  return (
    <form className="form-stack modal-body" onSubmit={submit}>
      <p className="modal-intro">
        Define the runtime image, install script, startup command, environment,
        ports, and server variables for this Config.
      </p>
      <div className="form-two">
        <Field label="Name">
          <input
            required
            value={name}
            onChange={(e) => changeName(e.target.value)}
          />
        </Field>
        <Field label="Slug">
          <input
            required
            value={slug}
            onChange={(e) => setSlug(e.target.value)}
          />
        </Field>
      </div>
      <Field label="Description">
        <input
          value={description}
          onChange={(e) => setDescription(e.target.value)}
        />
      </Field>
      {!config && availableConfigs.length > 0 && (
        <Field label="Copy specification from config">
          <select
            value={starterId}
            onChange={(e) => {
              setStarterId(e.target.value);
              const source = availableConfigs.find(
                (t) => t.id === e.target.value,
              );
              if (source) setSpec(JSON.stringify(source.spec, null, 2));
            }}
          >
            <option value="">Choose a config…</option>
            {availableConfigs.map((t) => (
              <option key={t.id} value={t.id}>
                {t.name}
              </option>
            ))}
          </select>
        </Field>
      )}
      <div className="config-editor-heading">
        <div>
          <strong>Config variables</strong>
          <small>
            Required values are collected when a server is created. Only
            selected optional values appear.
          </small>
        </div>
        <div
          className="config-editor-switch"
          role="tablist"
          aria-label="Config editor"
        >
          <button
            type="button"
            className={editorMode === "fields" ? "active" : ""}
            onClick={() => setEditorMode("fields")}
          >
            Fields
          </button>
          <button
            type="button"
            className={editorMode === "json" ? "active" : ""}
            onClick={() => setEditorMode("json")}
          >
            JSON
          </button>
        </div>
      </div>
      {editorMode === "fields" ? (
        parsed ? (
          <div className="config-variable-list">
            {vars.map((variable, index) => (
              <article
                className="config-variable-card"
                key={`${variable.name || "variable"}-${index}`}
              >
                <div className="form-two">
                  <Field label="Variable name">
                    <input
                      required
                      value={variable.name || ""}
                      placeholder="e.g. GAME_PORT"
                      onChange={(e) =>
                        updateVariable(index, { name: e.target.value })
                      }
                    />
                  </Field>
                  <Field label="Input type">
                    <select
                      value={variable.type || "text"}
                      onChange={(e) =>
                        updateVariable(index, { type: e.target.value })
                      }
                    >
                      <option value="text">Text</option>
                      <option value="integer">Number</option>
                      <option value="secret">Secret</option>
                    </select>
                  </Field>
                </div>
                <div className="form-two">
                  <Field label="Default value">
                    <input
                      type={
                        variable.type === "secret"
                          ? "password"
                          : variable.type === "integer"
                            ? "number"
                            : "text"
                      }
                      value={variable.default ?? ""}
                      onChange={(e) =>
                        updateVariable(index, { default: e.target.value })
                      }
                    />
                  </Field>
                  <Field label="Description">
                    <input
                      value={variable.description || ""}
                      onChange={(e) =>
                        updateVariable(index, {
                          description: e.target.value,
                        })
                      }
                    />
                  </Field>
                </div>
                <div className="config-variable-options">
                  <label>
                    <input
                      type="checkbox"
                      checked={variable.required === true}
                      onChange={(e) =>
                        updateVariable(index, {
                          required: e.target.checked,
                          ...(e.target.checked ? { ui: true } : {}),
                        })
                      }
                    />
                    Required when creating a server
                  </label>
                  <label>
                    <input
                      type="checkbox"
                      checked={
                        variable.required === true || variable.ui === true
                      }
                      disabled={variable.required === true}
                      onChange={(e) =>
                        updateVariable(index, { ui: e.target.checked })
                      }
                    />
                    Show in server form
                  </label>
                  <Button
                    variant="danger-outline"
                    size="small"
                    type="button"

                    onClick={() =>
                      updateVariables(vars.filter((_, i) => i !== index))
                    }
                  >
                    <Trash size={14} />
                    Remove
                  </Button>
                </div>
              </article>
            ))}
            <Button
              variant="subtle"
              type="button"

              onClick={() =>
                updateVariables([
                  ...vars,
                  {
                    name: "",
                    type: "text",
                    default: "",
                    required: false,
                    ui: false,
                  },
                ])
              }
            >
              <Plus size={15} />
              Add variable
            </Button>
            <small className="field-hint">
              Container image and startup settings remain available in the JSON
              editor.
            </small>
          </div>
        ) : (
          <div className="form-hint warning">
            The specification is not valid JSON. Switch to JSON to repair it.
          </div>
        )
      ) : (
        <div className="config-json-editor">
          <div className="config-json-toolbar">
            <span>{spec.split("\n").length} lines · JSON</span>
            <Button
              variant="subtle"
              size="small"
              type="button"

              onClick={() => {
                try {
                  setSpec(JSON.stringify(JSON.parse(spec), null, 2));
                } catch {
                  setError("Config specification must be valid JSON.");
                }
              }}
            >
              Format JSON
            </Button>
          </div>
          <CodeEditor
            value={spec}
            onChange={setSpec}
            language="json"
            ariaLabel="Config specification JSON"
            className="config-spec-editor"
          />
          <small className="field-hint">
            Include dockerImages and dataDirectory. Put downloads in the install
            script; startup runs inside the game container. Use
            {"{{VARIABLE}}"} placeholders in command and environment values.
          </small>
        </div>
      )}
      {error && <div className="form-error">{error}</div>}
      <FormActions
        cancel={onCancel}
        busy={busy}
        label={config ? "Save changes" : "Create config"}
      />
    </form>
  );
}
