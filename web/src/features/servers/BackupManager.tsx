import { Button } from "../../components/Button";
import { RotateCcw, Save, Trash } from "lucide-react";
import { api, errorMessage, type BackupItem, type GameServer } from "../../shared/domain";
import { formatBytes } from "../../components/formatBytes";
import { useConfirmation } from "../../shared/dialogs";
import { showToast } from "../../shared/toast";
import { useCallback, useEffect, useState } from "react";

/** List, create, restore, and remove backups for one server. */
export function BackupManager({
  server,
  canManage = false,
}: {
  server: GameServer;
  canManage?: boolean;
}) {
  const confirmAction = useConfirmation();
  const [items, setItems] = useState<BackupItem[]>([]);
  const [busy, setBusy] = useState(false);
  const load = useCallback(
    () =>
      api<BackupItem[]>(`/api/servers/${server.id}/backups`)
        .then((rows) => {
          setItems(rows || []);
        })
        .catch((e) => {
          setItems([]);
          showToast(errorMessage(e));
        }),
    [server.id],
  );
  useEffect(() => {
    void load();
  }, [load]);
  const create = async () => {
    setBusy(true);
    try {
      await api(`/api/servers/${server.id}/backups`, { method: "POST" });
      await load();
    } catch (e: unknown) {
      showToast(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };
  const restore = async (id: string) => {
    if (
      !(await confirmAction({
        title: "Restore backup",
        message:
          "The current server container will be replaced with this backup.",
        confirmLabel: "Restore backup",
        destructive: false,
      }))
    )
      return;
    setBusy(true);
    try {
      await api(`/api/servers/${server.id}/backups/${id}/restore`, {
        method: "POST",
      });
    } catch (e: unknown) {
      showToast(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };
  const remove = async (id: string) => {
    if (
      !(await confirmAction({
        title: "Delete backup",
        message: "This backup will be permanently removed from the node.",
        confirmLabel: "Delete backup",
        destructive: true,
      }))
    )
      return;
    setBusy(true);
    try {
      await api(`/api/servers/${server.id}/backups/${id}`, {
        method: "DELETE",
      });
      await load();
    } catch (e: unknown) {
      showToast(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };
  return (
    <section className="panel-card backups-card">
      <div className="file-toolbar">
        <strong>Container backups</strong>
        {canManage && (
          <Button
            variant="primary"
            size="tiny"

            disabled={!server.containerId || busy}
            onClick={create}
          >
            <Save size={14} />
            Create backup
          </Button>
        )}
      </div>
      {items.map((item) => (
        <div className="backup-row" key={item.id}>
          <div>
            <strong>{new Date(item.createdAt).toLocaleString()}</strong>
            <small>{item.id}</small>
            <small>{formatBytes(item.sizeBytes || 0)}</small>
          </div>
          {canManage && (
            <Button
              variant="subtle"
              size="tiny"

              disabled={busy}
              onClick={() => restore(item.id)}
            >
              <RotateCcw size={14} />
              Restore
            </Button>
          )}
          {canManage && (
            <Button
              variant="danger"
              size="tiny"

              disabled={busy}
              onClick={() => remove(item.id)}
            >
              <Trash size={14} />
              Delete
            </Button>
          )}
        </div>
      ))}
      {!items.length && (
        <div className="console-placeholder">No backups yet.</div>
      )}
    </section>
  );
}
