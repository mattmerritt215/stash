import React, { createContext, useContext, useMemo } from "react";
import * as GQL from "src/core/generated-graphql";

/**
 * Permission slugs mirrored from pkg/auth/roles.go. Keep the two in sync:
 * the server is the enforcement point, this is only what the UI uses to
 * decide what to render.
 */
export type Permission =
  | "view_library"
  | "track_activity"
  | "personalise"
  | "edit_metadata"
  | "delete_content"
  | "scrape"
  | "run_tasks"
  | "manage_server";

export interface IPermissionContext {
  loading: boolean;
  /** Null when the multi-user layer is disabled. */
  user: GQL.MeQuery["me"]["user"] | null;
  permissions: Set<Permission>;
  authMode: string;
  logoutUrl: string;
  can: (p: Permission) => boolean;
  isAdmin: boolean;
  multiUser: boolean;
}

const allPermissions: Permission[] = [
  "view_library",
  "track_activity",
  "personalise",
  "edit_metadata",
  "delete_content",
  "scrape",
  "run_tasks",
  "manage_server",
];

const defaultContext: IPermissionContext = {
  loading: true,
  user: null,
  permissions: new Set(allPermissions),
  authMode: "disabled",
  logoutUrl: "/logout",
  can: () => true,
  isAdmin: true,
  multiUser: false,
};

export const PermissionContext =
  createContext<IPermissionContext>(defaultContext);

export const PermissionProvider: React.FC<React.PropsWithChildren<{}>> = ({
  children,
}) => {
  const { data, loading } = GQL.useMeQuery({
    fetchPolicy: "cache-first",
  });

  const value = useMemo<IPermissionContext>(() => {
    // Until the query resolves, and if it fails outright, assume full
    // permissions. The server enforces the real thing; assuming otherwise
    // would flash an empty UI at an administrator on every page load.
    if (loading || !data?.me) {
      return { ...defaultContext, loading };
    }

    const me = data.me;
    const permissions = new Set(me.permissions as Permission[]);

    return {
      loading: false,
      user: me.user ?? null,
      permissions,
      authMode: me.auth_mode,
      logoutUrl: me.logout_url,
      can: (p: Permission) => permissions.has(p),
      isAdmin: permissions.has("manage_server"),
      multiUser: me.auth_mode !== "disabled",
    };
  }, [data, loading]);

  return (
    <PermissionContext.Provider value={value}>
      {children}
    </PermissionContext.Provider>
  );
};

export const usePermissions = () => useContext(PermissionContext);

export const useHasPermission = (p: Permission) => usePermissions().can(p);

/** Renders its children only when the caller holds the permission. */
export const Gate: React.FC<
  React.PropsWithChildren<{ perm: Permission; fallback?: React.ReactNode }>
> = ({ perm, fallback = null, children }) => {
  const { can } = usePermissions();
  return <>{can(perm) ? children : fallback}</>;
};

/**
 * Which permission each settings tab requires. Tabs the user cannot use are
 * removed from the nav and their panes are not mounted.
 *
 * "interface" is deliberately open to everyone with `personalise`: that is
 * the per-user customisation surface, and the server stores those writes
 * against the calling user rather than the server config.
 */
export const settingsTabPermissions: Record<string, Permission> = {
  tasks: "run_tasks",
  library: "manage_server",
  interface: "personalise",
  security: "manage_server",
  "metadata-providers": "scrape",
  services: "manage_server",
  system: "manage_server",
  plugins: "manage_server",
  logs: "manage_server",
  tools: "manage_server",
  users: "manage_server",
  changelog: "view_library",
  about: "view_library",
};

function eventKeyOf(node: React.ReactNode): string | undefined {
  if (!React.isValidElement(node)) return undefined;

  const props = node.props as Record<string, unknown>;
  if (typeof props.eventKey === "string") {
    return props.eventKey;
  }

  const children = props.children as React.ReactNode;
  if (children === undefined || children === null) return undefined;

  let found: string | undefined;
  React.Children.forEach(children, (child) => {
    if (found === undefined) {
      found = eventKeyOf(child);
    }
  });
  return found;
}

/**
 * Drops any child whose (possibly nested) eventKey names a settings tab the
 * caller may not use. Wrapping the nav and the tab content in this keeps the
 * gating in one place instead of scattering conditionals through the JSX,
 * which matters for rebasing onto upstream.
 */
export const SettingsTabGate: React.FC<React.PropsWithChildren<{}>> = ({
  children,
}) => {
  const { can } = usePermissions();

  const filtered = React.Children.toArray(children).filter((child) => {
    const key = eventKeyOf(child);
    if (key === undefined) return true;
    const required = settingsTabPermissions[key];
    if (required === undefined) return true;
    return can(required);
  });

  return <>{filtered}</>;
};

/** The first settings tab this user is allowed to see. */
export function firstAllowedSettingsTab(
  can: (p: Permission) => boolean,
  order: readonly string[]
): string {
  for (const tab of order) {
    const required = settingsTabPermissions[tab];
    if (required === undefined || can(required)) return tab;
  }
  return "about";
}
