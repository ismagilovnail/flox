import { CheckIcon } from "lucide-react";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Card, CardContent } from "@/components/ui/card";
import { Mono } from "@/components/ui/typography";
import { PERMISSIONS, ROLES, ROLE_PERMISSIONS } from "@/lib/mock/team";

export function RolesPermissionsPanel() {
  return (
    <div className="flex flex-col gap-4">
      <Alert>
        <AlertDescription>
          Fixed roles, not custom ones — matches §52. Enforcement is server-side (this Team page itself is
          permission-gated by it); other domains don&apos;t check permissions yet, only tenant isolation.
        </AlertDescription>
      </Alert>

      <Card>
        <CardContent className="overflow-x-auto p-0">
          <table className="w-full min-w-[640px] border-collapse text-sm">
            <thead>
              <tr className="border-b border-border">
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">Permission</th>
                {ROLES.map((role) => (
                  <th key={role} className="px-3 py-2.5 text-center font-medium text-muted-foreground">
                    {role}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {PERMISSIONS.map((permission) => (
                <tr key={permission} className="border-b border-border last:border-0">
                  <td className="px-4 py-2">
                    <Mono className="text-xs">{permission}</Mono>
                  </td>
                  {ROLES.map((role) => (
                    <td key={role} className="px-3 py-2 text-center">
                      {ROLE_PERMISSIONS[role].includes(permission) && (
                        <CheckIcon className="mx-auto size-3.5 text-success" />
                      )}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </CardContent>
      </Card>
    </div>
  );
}
