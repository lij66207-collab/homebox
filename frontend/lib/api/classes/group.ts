import { BaseAPI, route } from "../base";
import type {
  CurrenciesCurrency,
  Group,
  GroupAcceptInvitationResponse,
  GroupInvitation,
  GroupInvitationCreate,
  GroupTestServerChanRequest,
  GroupUpdate,
  UserSummary,
} from "../types/data-contracts";
import type { Result } from "../types/non-generated";

/**
 * AssistantSettings is the shape of the `assistant` namespace in the group
 * settings JSON. `stt_api_key` is redacted by the server on reads ("REDACTED").
 */
export interface AssistantSettings {
  enabled: boolean;
  stt_base_url: string;
  stt_api_key: string;
  stt_model: string;
}

/**
 * ExpiryReminderSettings is the shape of the `expiry_reminder` namespace in the
 * group settings JSON. `sendkey` is redacted by the server on reads.
 */
export interface ExpiryReminderSettings {
  enabled: boolean;
  sendkey: string;
  days_before: number[];
  notify_hour: number;
}

export class GroupApi extends BaseAPI {
  /**
   * Create a new invitation for the current group.
   */
  createInvitation(data: GroupInvitationCreate) {
    return this.http.post<GroupInvitationCreate, GroupInvitation>({
      url: route("/groups/invitations"),
      body: data,
    });
  }

  /**
   * Accept an invitation.
   */
  acceptInvitation(id: string) {
    return this.http.post<null, GroupAcceptInvitationResponse>({
      url: route(`/groups/invitations/${id}`),
    });
  }

  /**
   * Get all invitations for the current group.
   */
  getInvitations() {
    return this.http.get<GroupInvitation[]>({
      url: route("/groups/invitations"),
    });
  }

  /**
   * Delete an invitation by ID.
   */
  deleteInvitation(id: string) {
    return this.http.delete<void>({
      url: route(`/groups/invitations/${id}`),
    });
  }

  /**
   * Get all members of the current (or specified) group.
   */
  getMembers(groupId?: string) {
    const headers = groupId
      ? {
          "X-Tenant": groupId,
        }
      : undefined;
    return this.http.get<UserSummary[]>({
      url: route(`/groups/members`),
      headers,
    });
  }

  /**
   * Remove a user from the current (or specified) group.
   */
  removeMember(userId: string, groupId?: string) {
    const headers = groupId
      ? {
          "X-Tenant": groupId,
        }
      : undefined;
    return this.http.delete<void>({
      url: route(`/groups/members/${userId}`),
      headers,
    });
  }

  /**
   * Update group name and currency.
   */
  update(data: GroupUpdate, groupId?: string) {
    const headers = groupId
      ? {
          "X-Tenant": groupId,
        }
      : undefined;
    return this.http.put<GroupUpdate, Group>({
      url: route(`/groups`),
      headers,
      body: data,
    });
  }

  /**
   * Get a group by ID, if no ID is provided, get the current group.
   */
  get(groupId?: string) {
    const headers = groupId
      ? {
          "X-Tenant": groupId,
        }
      : undefined;
    return this.http.get<Group>({
      url: route(`/groups`),
      headers,
    });
  }

  /**
   * Get all groups the user is a member of.
   */
  getAll() {
    return this.http.get<Group[]>({
      url: route("/groups/all"),
    });
  }

  /**
   * Create a new group with the given name.
   */
  create(name: string) {
    return this.http.post<
      {
        name: string;
      },
      Group
    >({
      url: route("/groups"),
      body: { name },
    });
  }

  /**
   * Delete a group by ID, if no ID is provided, delete the current group.
   */
  delete(groupId?: string) {
    const headers = groupId
      ? {
          "X-Tenant": groupId,
        }
      : undefined;
    return this.http.delete<void>({
      url: route(`/groups`),
      headers,
    });
  }

  /**
   * Get all currencies.
   */
  currencies() {
    return this.http.get<CurrenciesCurrency[]>({
      url: route("/currencies"),
    });
  }

  /**
   * Get the raw group settings JSON (sensitive values are redacted by the
   * server, e.g. "REDACTED").
   */
  getSettings() {
    return this.http.get<Result<Record<string, unknown>>>({
      url: route("/group/settings"),
    });
  }

  /**
   * Replace the group settings JSON. Sensitive keys sent back as "REDACTED"
   * (or "") keep their stored values server-side.
   */
  updateSettings(settings: Record<string, unknown>) {
    return this.http.put<Record<string, unknown>, Result<Record<string, unknown>>>({
      url: route("/group/settings"),
      body: settings,
    });
  }

  /**
   * Send a Server酱 test push. When `sendkey` is empty, the stored one is used.
   */
  testServerChan(sendkey = "") {
    return this.http.post<GroupTestServerChanRequest, Result<string>>({
      url: route("/group/settings/test-serverchan"),
      body: { sendkey },
    });
  }
}
