<script setup lang="ts">
  import { useI18n } from "vue-i18n";
  import { toast } from "@/components/ui/sonner";
  import MdiShieldAccount from "~icons/mdi/shield-account";
  import MdiPlus from "~icons/mdi/plus";
  import MdiLinkVariant from "~icons/mdi/link-variant";
  import MdiDelete from "~icons/mdi/delete";
  import MdiLoading from "~icons/mdi/loading";
  import MdiKeyVariant from "~icons/mdi/key-variant";
  import MdiAccountOff from "~icons/mdi/account-off";
  import MdiAccountCheck from "~icons/mdi/account-check";
  import { Button } from "@/components/ui/button";
  import { Badge } from "@/components/ui/badge";
  import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
  import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
  import { Input } from "@/components/ui/input";
  import BaseContainer from "@/components/Base/Container.vue";
  import BaseCard from "@/components/Base/Card.vue";
  import BaseSectionHeader from "@/components/Base/SectionHeader.vue";
  import FormTextField from "~/components/Form/TextField.vue";
  import FormPassword from "~/components/Form/Password.vue";
  import PasswordScore from "~/components/global/PasswordScore.vue";
  import CopyText from "~/components/global/CopyText.vue";
  import { PASSWORD_MIN_LENGTH, PASSWORD_RULES } from "~/lib/passwords";
  import type { UserOut } from "~~/lib/api/types/data-contracts";

  const { t } = useI18n();

  definePageMeta({
    middleware: [
      "auth",
      () => {
        const ctx = useAuthContext();
        if (!ctx.user?.isSuperuser) {
          return navigateTo("/home");
        }
      },
    ],
  });

  useHead({
    title: "LJJ Organizer | " + t("menu.admin"),
  });

  const api = useUserApi();
  const confirm = useConfirm();
  const auth = useAuthContext();

  const users = ref<UserOut[]>([]);
  const loading = ref(true);

  async function loadUsers() {
    loading.value = true;
    const { data, error } = await api.admin.listUsers();
    loading.value = false;
    if (error) {
      toast.error(t("admin.toast.load_failed"));
      return;
    }
    users.value = data.items;
  }

  onMounted(loadUsers);

  // ---- create user ----
  const createOpen = ref(false);
  const createLoading = ref(false);
  const createForm = reactive({ name: "", email: "", password: "" });
  const createPasswordValid = ref(false);

  function openCreate() {
    createForm.name = "";
    createForm.email = "";
    createForm.password = "";
    createPasswordValid.value = false;
    createOpen.value = true;
  }

  async function submitCreate() {
    createLoading.value = true;
    const { error } = await api.admin.createUser({
      name: createForm.name,
      email: createForm.email,
      password: createForm.password,
    });
    createLoading.value = false;

    if (error) {
      toast.error(t("admin.toast.create_failed"));
      return;
    }

    toast.success(t("admin.toast.created"));
    createOpen.value = false;
    await loadUsers();
  }

  // ---- reset link ----
  const resetOpen = ref(false);
  const resetLink = ref("");
  const resetLoading = ref(false);

  async function generateResetLink(user: UserOut) {
    resetLoading.value = true;
    const { data, error } = await api.admin.resetLink(user.id);
    resetLoading.value = false;

    if (error) {
      toast.error(t("admin.toast.reset_failed"));
      return;
    }

    resetLink.value = data.item.link;
    resetOpen.value = true;
  }

  // ---- delete ----
  async function deleteUser(user: UserOut) {
    const result = await confirm.open(t("admin.delete_confirm", { email: user.email }));
    if (result.isCanceled) {
      return;
    }

    const { error } = await api.admin.deleteUser(user.id);
    if (error) {
      toast.error(t("admin.toast.delete_failed"));
      return;
    }

    toast.success(t("admin.toast.deleted"));
    await loadUsers();
  }

  // ---- search ----
  const search = ref("");
  const filteredUsers = computed(() => {
    const q = search.value.trim().toLowerCase();
    if (!q) {
      return users.value;
    }
    return users.value.filter(u => u.name.toLowerCase().includes(q) || u.email.toLowerCase().includes(q));
  });

  // ---- set password ----
  const passwordOpen = ref(false);
  const passwordLoading = ref(false);
  const passwordTarget = ref<UserOut | null>(null);
  const newPassword = ref("");
  const newPasswordValid = ref(false);

  function openSetPassword(user: UserOut) {
    passwordTarget.value = user;
    newPassword.value = "";
    newPasswordValid.value = false;
    passwordOpen.value = true;
  }

  async function submitSetPassword() {
    if (!passwordTarget.value) {
      return;
    }
    passwordLoading.value = true;
    const { error } = await api.admin.setPassword(passwordTarget.value.id, newPassword.value);
    passwordLoading.value = false;

    if (error) {
      toast.error(t("admin.toast.password_failed"));
      return;
    }

    toast.success(t("admin.toast.password_set"));
    passwordOpen.value = false;
  }

  // ---- disable / enable ----
  async function toggleDisabled(user: UserOut) {
    if (!user.disabled) {
      const result = await confirm.open(t("admin.disable_confirm", { email: user.email }));
      if (result.isCanceled) {
        return;
      }
    }

    const { error } = await api.admin.setDisabled(user.id, !user.disabled);
    if (error) {
      toast.error(t("admin.toast.status_failed"));
      return;
    }

    toast.success(t("admin.toast.status_updated"));
    await loadUsers();
  }
</script>

<template>
  <BaseContainer>
    <BaseSectionHeader class="mb-2 flex items-center justify-between">
      <span class="flex items-center gap-2">
        <MdiShieldAccount class="size-6 text-primary" />
        {{ $t("admin.title") }}
      </span>
      <template #description>
        <Button size="sm" class="rounded-full" @click="openCreate">
          <MdiPlus />
          {{ $t("admin.create_user") }}
        </Button>
      </template>
    </BaseSectionHeader>

    <BaseCard>
      <div class="p-2">
        <div class="p-2">
          <Input v-model:model-value="search" :placeholder="$t('admin.search_placeholder')" class="max-w-xs" />
        </div>
        <div v-if="loading" class="flex justify-center py-8">
          <MdiLoading class="size-8 animate-spin text-muted-foreground" />
        </div>
        <Table v-else>
          <TableHeader>
            <TableRow>
              <TableHead>{{ $t("global.name") }}</TableHead>
              <TableHead>{{ $t("global.email") }}</TableHead>
              <TableHead class="text-right">{{ $t("admin.actions") }}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="user in filteredUsers" :key="user.id">
              <TableCell class="font-medium">
                <span class="flex items-center gap-2">
                  {{ user.name }}
                  <Badge v-if="user.isSuperuser" variant="secondary">{{ $t("admin.superuser") }}</Badge>
                  <Badge v-if="user.disabled" variant="destructive">{{ $t("admin.disabled") }}</Badge>
                </span>
              </TableCell>
              <TableCell>{{ user.email }}</TableCell>
              <TableCell class="text-right">
                <div class="flex items-center justify-end gap-1">
                  <Button
                    size="icon"
                    variant="ghost"
                    class="rounded-full"
                    :aria-label="$t('admin.set_password')"
                    @click="openSetPassword(user)"
                  >
                    <MdiKeyVariant class="size-4" />
                  </Button>
                  <Button
                    size="icon"
                    variant="ghost"
                    class="rounded-full"
                    :disabled="user.id === auth.user?.id"
                    :aria-label="user.disabled ? $t('admin.enable') : $t('admin.disable')"
                    @click="toggleDisabled(user)"
                  >
                    <MdiAccountCheck v-if="user.disabled" class="size-4" />
                    <MdiAccountOff v-else class="size-4" />
                  </Button>
                  <Button
                    size="icon"
                    variant="ghost"
                    class="rounded-full"
                    :disabled="resetLoading"
                    :aria-label="$t('admin.reset_link')"
                    @click="generateResetLink(user)"
                  >
                    <MdiLinkVariant class="size-4" />
                  </Button>
                  <Button
                    size="icon"
                    variant="ghost"
                    class="rounded-full text-destructive hover:bg-destructive/10"
                    :disabled="user.id === auth.user?.id"
                    :aria-label="$t('admin.delete_user')"
                    @click="deleteUser(user)"
                  >
                    <MdiDelete class="size-4" />
                  </Button>
                </div>
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </div>
    </BaseCard>

    <!-- create user dialog -->
    <Dialog v-model:open="createOpen">
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{{ $t("admin.create_user") }}</DialogTitle>
        </DialogHeader>
        <form id="admin-create-user" class="flex flex-col gap-2" @submit.prevent="submitCreate">
          <FormTextField v-model="createForm.name" :label="$t('global.name')" :required="true" />
          <FormTextField v-model="createForm.email" :label="$t('global.email')" type="email" :required="true" />
          <FormPassword
            v-model="createForm.password"
            :label="$t('global.password')"
            :min-length="PASSWORD_MIN_LENGTH"
            :passwordrules="PASSWORD_RULES"
            :required="true"
          />
          <PasswordScore v-model:valid="createPasswordValid" :password="createForm.password" />
        </form>
        <DialogFooter>
          <Button
            form="admin-create-user"
            type="submit"
            :disabled="createLoading || !createPasswordValid || !createForm.name || !createForm.email"
          >
            <MdiLoading v-if="createLoading" class="animate-spin" />
            {{ $t("global.create") }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <!-- reset link dialog -->
    <Dialog v-model:open="resetOpen">
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{{ $t("admin.reset_link_title") }}</DialogTitle>
        </DialogHeader>
        <p class="text-sm text-muted-foreground">{{ $t("admin.reset_link_sub") }}</p>
        <div class="flex items-center gap-2">
          <Input
            :model-value="resetLink"
            readonly
            class="text-xs"
            @focus="($event.target as HTMLInputElement).select()"
          />
          <CopyText :text="resetLink" :icon-size="18" />
        </div>
        <DialogFooter>
          <Button variant="secondary" @click="resetOpen = false">{{ $t("global.close") }}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <!-- set password dialog -->
    <Dialog v-model:open="passwordOpen">
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{{ $t("admin.set_password_title", { name: passwordTarget?.name }) }}</DialogTitle>
        </DialogHeader>
        <form id="admin-set-password" class="flex flex-col gap-2" @submit.prevent="submitSetPassword">
          <FormPassword
            v-model="newPassword"
            :label="$t('admin.new_password')"
            :min-length="PASSWORD_MIN_LENGTH"
            :passwordrules="PASSWORD_RULES"
            :required="true"
          />
          <PasswordScore v-model:valid="newPasswordValid" :password="newPassword" />
        </form>
        <DialogFooter>
          <Button form="admin-set-password" type="submit" :disabled="passwordLoading || !newPasswordValid">
            <MdiLoading v-if="passwordLoading" class="animate-spin" />
            {{ $t("global.submit") }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </BaseContainer>
</template>
