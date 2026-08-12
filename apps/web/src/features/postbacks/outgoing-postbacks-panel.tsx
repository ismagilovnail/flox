import { Alert, AlertDescription } from "@/components/ui/alert";
import { NetworkList } from "@/features/networks/network-list";

/** Outgoing postbacks ARE the Network entity's postbackUrl/acceptDuplicates fields
 * (§27, extended §45) — reusing NetworkList here instead of a second table keeps
 * one source of truth for network config rather than duplicating the CRUD. */
export function OutgoingPostbacksPanel() {
  return (
    <div className="flex flex-col gap-4">
      <Alert>
        <AlertDescription>
          FLOX calls a network&apos;s postback URL whenever a conversion status changes for that network. Managed
          here and on the Networks page — they&apos;re the same data.
        </AlertDescription>
      </Alert>
      <NetworkList />
    </div>
  );
}
