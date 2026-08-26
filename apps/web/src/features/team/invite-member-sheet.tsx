"use client";

import { Controller, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { ROLES, type Role } from "@/lib/mock/team";

const INVITABLE_ROLES = ROLES.filter((r) => r !== "Owner") as [Role, ...Role[]];

export const inviteMemberSchema = z.object({
  name: z.string().min(2, "Name must be at least 2 characters").max(80),
  email: z.email("Enter a valid email"),
  role: z.enum(INVITABLE_ROLES),
});

export type InviteMemberValues = z.infer<typeof inviteMemberSchema>;

export function InviteMemberSheet({
  open,
  onOpenChange,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (values: InviteMemberValues) => void;
}) {
  const form = useForm<InviteMemberValues>({
    resolver: zodResolver(inviteMemberSchema),
    defaultValues: { name: "", email: "", role: "Analyst" },
  });

  const {
    register,
    handleSubmit,
    control,
    formState: { errors, isSubmitting },
  } = form;

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full gap-0 overflow-y-auto sm:max-w-md" side="right">
        <SheetHeader>
          <SheetTitle>Invite team member</SheetTitle>
          <SheetDescription>
            You&apos;ll get a link to send them yourself — there&apos;s no email delivery yet. Roles determine what
            they can see and edit (§52).
          </SheetDescription>
        </SheetHeader>

        <form onSubmit={handleSubmit(onSubmit)} className="flex flex-col gap-4 px-4 pb-4">
          <div className="grid gap-1.5">
            <Label htmlFor="inv-name">Name</Label>
            <Input id="inv-name" placeholder="Jordan Lee" {...register("name")} aria-invalid={!!errors.name} />
            {errors.name && <p className="text-xs text-danger">{errors.name.message}</p>}
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="inv-email">Email</Label>
            <Input
              id="inv-email"
              type="email"
              placeholder="jordan@company.com"
              {...register("email")}
              aria-invalid={!!errors.email}
            />
            {errors.email && <p className="text-xs text-danger">{errors.email.message}</p>}
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="inv-role">Role</Label>
            <Controller
              control={control}
              name="role"
              render={({ field }) => (
                <Select value={field.value} onValueChange={field.onChange}>
                  <SelectTrigger id="inv-role" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {INVITABLE_ROLES.map((r) => (
                      <SelectItem key={r} value={r}>
                        {r}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            />
          </div>

          <SheetFooter className="mt-0 flex-row justify-end gap-2 p-0">
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={isSubmitting}>
              Send invite
            </Button>
          </SheetFooter>
        </form>
      </SheetContent>
    </Sheet>
  );
}
