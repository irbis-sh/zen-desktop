
/*
        this file contains code from the systray project
   (https://github.com/getlantern/systray), licensed under the apache license.
        see more in the copying.md file in the root directory of this project.
*/

#include <dlfcn.h>
#include <errno.h>
#include <limits.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#include <gtk/gtk.h>

#include "systray.h"

// The AppIndicator library (libayatana-appindicator3 or the legacy
// libappindicator3) is loaded at runtime via dlopen instead of being linked at
// build time. This keeps it an optional dependency: machines without the
// library can still launch the app, just without the tray icon. See
// try_load_appindicator below.
//
// AppIndicator is treated as an opaque pointer; we never dereference it. The
// enum constants below are part of the library's stable ABI and are identical
// across the ayatana and legacy variants.
typedef struct _AppIndicator AppIndicator;

#define APP_INDICATOR_CATEGORY_APPLICATION_STATUS 0
#define APP_INDICATOR_STATUS_PASSIVE 0
#define APP_INDICATOR_STATUS_ACTIVE 1

typedef AppIndicator *(*app_indicator_new_func)(const gchar *id,
                                                const gchar *icon_name,
                                                int category);
typedef void (*app_indicator_set_status_func)(AppIndicator *self, int status);
typedef void (*app_indicator_set_menu_func)(AppIndicator *self, GtkMenu *menu);
typedef void (*app_indicator_set_icon_full_func)(AppIndicator *self,
                                                 const gchar *icon_name,
                                                 const gchar *icon_desc);
typedef void (*app_indicator_set_attention_icon_full_func)(
    AppIndicator *self, const gchar *icon_name, const gchar *icon_desc);
typedef void (*app_indicator_set_title_func)(AppIndicator *self,
                                             const gchar *title);
typedef void (*app_indicator_set_label_func)(AppIndicator *self,
                                             const gchar *label,
                                             const gchar *guide);

static app_indicator_new_func p_app_indicator_new = NULL;
static app_indicator_set_status_func p_app_indicator_set_status = NULL;
static app_indicator_set_menu_func p_app_indicator_set_menu = NULL;
static app_indicator_set_icon_full_func p_app_indicator_set_icon_full = NULL;
static app_indicator_set_attention_icon_full_func
    p_app_indicator_set_attention_icon_full = NULL;
static app_indicator_set_title_func p_app_indicator_set_title = NULL;
static app_indicator_set_label_func p_app_indicator_set_label = NULL;

// resolve_appindicator_symbols resolves all required app_indicator_* symbols
// from handle into the global function pointers. Returns 1 only if every symbol
// is found; otherwise it resets the pointers back to NULL and returns 0, so a
// partially-resolved handle never leaves dangling pointers behind.
static int resolve_appindicator_symbols(void *handle) {
  // The pointer-to-pointer cast avoids the ISO C warning about converting an
  // object pointer (dlsym's return) to a function pointer.
  *(void **)(&p_app_indicator_new) = dlsym(handle, "app_indicator_new");
  *(void **)(&p_app_indicator_set_status) =
      dlsym(handle, "app_indicator_set_status");
  *(void **)(&p_app_indicator_set_menu) = dlsym(handle, "app_indicator_set_menu");
  *(void **)(&p_app_indicator_set_icon_full) =
      dlsym(handle, "app_indicator_set_icon_full");
  *(void **)(&p_app_indicator_set_attention_icon_full) =
      dlsym(handle, "app_indicator_set_attention_icon_full");
  *(void **)(&p_app_indicator_set_title) =
      dlsym(handle, "app_indicator_set_title");
  *(void **)(&p_app_indicator_set_label) =
      dlsym(handle, "app_indicator_set_label");

  if (p_app_indicator_new == NULL || p_app_indicator_set_status == NULL ||
      p_app_indicator_set_menu == NULL || p_app_indicator_set_icon_full == NULL ||
      p_app_indicator_set_attention_icon_full == NULL ||
      p_app_indicator_set_title == NULL || p_app_indicator_set_label == NULL) {
    p_app_indicator_new = NULL;
    p_app_indicator_set_status = NULL;
    p_app_indicator_set_menu = NULL;
    p_app_indicator_set_icon_full = NULL;
    p_app_indicator_set_attention_icon_full = NULL;
    p_app_indicator_set_title = NULL;
    p_app_indicator_set_label = NULL;
    return 0;
  }
  return 1;
}

// try_load_appindicator attempts to dlopen the AppIndicator library (preferring
// the ayatana variant, falling back to the legacy one) and resolve the symbols
// we need. It caches its result on the first call. It only performs dlopen/dlsym
// and never touches GTK, so it is safe to call before gtk_init.
//
// It is NOT safe for concurrent first-time calls: the cache and the resolved
// function pointers are written without synchronization. It must be primed once
// on a single thread before any concurrent use. In practice this holds because
// systray.Available() (via trayLibraryAvailable) runs to completion on the main
// goroutine during wails.Run setup, before the GTK loop dispatches
// do_register_systray. Returns 1 if the library and all symbols are available,
// 0 otherwise.
int try_load_appindicator(void) {
  static int tried = 0;
  static int ok = 0;
  if (tried) {
    return ok;
  }
  tried = 1;

  // Only ever load the gtk3 ("-3") sonames. Loading the gtk2 libappindicator
  // into this gtk3 process would be catastrophic.
  static const char *const sonames[] = {
      "libayatana-appindicator3.so.1",
      "libappindicator3.so.1",
  };
  for (size_t i = 0; i < sizeof(sonames) / sizeof(sonames[0]); i++) {
    void *handle = dlopen(sonames[i], RTLD_NOW | RTLD_LOCAL);
    if (handle == NULL) {
      continue;
    }
    if (resolve_appindicator_symbols(handle)) {
      ok = 1;
      return 1;
    }
    // Present but missing a required symbol; release it and try the next one.
    dlclose(handle);
  }

  printf("systray: AppIndicator library not available; tray icon disabled\n");
  return 0;
}

static AppIndicator *global_app_indicator;
static GtkWidget *global_tray_menu = NULL;
static GList *global_menu_items = NULL;
static char temp_file_name[PATH_MAX] = "";

typedef struct {
  GtkWidget *menu_item;
  int menu_id;
  long signalHandlerId;
} MenuItemNode;

typedef struct {
  int menu_id;
  int parent_menu_id;
  char *title;
  char *tooltip;
  short disabled;
  short checked;
  short isCheckable;
} MenuItemInfo;

gboolean do_register_systray(gpointer data) {
  // If the AppIndicator library is unavailable, skip creating the tray
  // entirely. We must not call systray_ready() in that case, so the Go side
  // never tries to build menu items against a non-existent tray.
  if (!try_load_appindicator()) {
    return FALSE;
  }
  global_app_indicator = p_app_indicator_new(
      "systray", "", APP_INDICATOR_CATEGORY_APPLICATION_STATUS);
  p_app_indicator_set_status(global_app_indicator, APP_INDICATOR_STATUS_ACTIVE);
  global_tray_menu = gtk_menu_new();
  p_app_indicator_set_menu(global_app_indicator, GTK_MENU(global_tray_menu));
  systray_ready();
  return FALSE;
}

void registerSystray(void) { g_idle_add(do_register_systray, NULL); }

int nativeLoop(void) {
  systray_on_exit();
  return 0;
}

void _unlink_temp_file() {
  if (strlen(temp_file_name) != 0) {
    int ret = unlink(temp_file_name);
    if (ret == -1) {
      printf("failed to remove temp icon file %s: %s\n", temp_file_name,
             strerror(errno));
    }
    temp_file_name[0] = '\0';
  }
}

// runs in main thread, should always return FALSE to prevent gtk to execute it
// again
gboolean do_set_icon(gpointer data) {
  GBytes *bytes = (GBytes *)data;
  // The tray may not have been created (library unavailable). Guard before
  // touching the filesystem so we never leave an orphan temp file behind.
  if (global_app_indicator == NULL) {
    g_bytes_unref(bytes);
    return FALSE;
  }
  _unlink_temp_file();
  char *tmpdir = getenv("TMPDIR");
  if (NULL == tmpdir) {
    tmpdir = "/tmp";
  }
  strncpy(temp_file_name, tmpdir, PATH_MAX - 1);
  strncat(temp_file_name, "/systray_XXXXXX", PATH_MAX - 1);
  temp_file_name[PATH_MAX - 1] = '\0';

  int fd = mkstemp(temp_file_name);
  if (fd == -1) {
    printf("failed to create temp icon file %s: %s\n", temp_file_name,
           strerror(errno));
    g_bytes_unref(bytes);
    return FALSE;
  }
  gsize size = 0;
  gconstpointer icon_data = g_bytes_get_data(bytes, &size);
  ssize_t written = write(fd, icon_data, size);
  close(fd);
  if (written != size) {
    printf("failed to write temp icon file %s: %s\n", temp_file_name,
           strerror(errno));
    _unlink_temp_file();
    g_bytes_unref(bytes);
    return FALSE;
  }
  p_app_indicator_set_icon_full(global_app_indicator, temp_file_name, "");
  p_app_indicator_set_attention_icon_full(global_app_indicator, temp_file_name,
                                          "");
  g_bytes_unref(bytes);
  return FALSE;
}

void _systray_menu_item_selected(gpointer data) {
  systray_menu_item_selected(GPOINTER_TO_INT(data));
}

GtkMenuItem *find_menu_by_id(int id) {
  GList *it;
  for (it = global_menu_items; it != NULL; it = it->next) {
    MenuItemNode *item = (MenuItemNode *)(it->data);
    if (item->menu_id == id) {
      return GTK_MENU_ITEM(item->menu_item);
    }
  }
  return NULL;
}

// runs in main thread, should always return FALSE to prevent gtk to execute it
// again
gboolean do_add_or_update_menu_item(gpointer data) {
  MenuItemInfo *mii = (MenuItemInfo *)data;
  GList *it;
  for (it = global_menu_items; it != NULL; it = it->next) {
    MenuItemNode *item = (MenuItemNode *)(it->data);
    if (item->menu_id == mii->menu_id) {
      gtk_menu_item_set_label(GTK_MENU_ITEM(item->menu_item), mii->title);

      if (mii->isCheckable) {
        // We need to block the "activate" event, to emulate the same behaviour
        // as in the windows version A Check/Uncheck does change the checkbox,
        // but does not trigger the checkbox menuItem channel
        g_signal_handler_block(GTK_CHECK_MENU_ITEM(item->menu_item),
                               item->signalHandlerId);
        gtk_check_menu_item_set_active(GTK_CHECK_MENU_ITEM(item->menu_item),
                                       mii->checked == 1);
        g_signal_handler_unblock(GTK_CHECK_MENU_ITEM(item->menu_item),
                                 item->signalHandlerId);
      }
      break;
    }
  }

  // menu id doesn't exist, add new item
  if (it == NULL) {
    GtkWidget *menu_item;
    if (mii->isCheckable) {
      menu_item = gtk_check_menu_item_new_with_label(mii->title);
      gtk_check_menu_item_set_active(GTK_CHECK_MENU_ITEM(menu_item),
                                     mii->checked == 1);
    } else {
      menu_item = gtk_menu_item_new_with_label(mii->title);
    }
    long signalHandlerId = g_signal_connect_swapped(
        G_OBJECT(menu_item), "activate",
        G_CALLBACK(_systray_menu_item_selected), GINT_TO_POINTER(mii->menu_id));

    if (mii->parent_menu_id == 0) {
      gtk_menu_shell_append(GTK_MENU_SHELL(global_tray_menu), menu_item);
    } else {
      GtkMenuItem *parentMenuItem = find_menu_by_id(mii->parent_menu_id);
      GtkWidget *parentMenu = gtk_menu_item_get_submenu(parentMenuItem);

      if (parentMenu == NULL) {
        parentMenu = gtk_menu_new();
        gtk_menu_item_set_submenu(parentMenuItem, parentMenu);
      }

      gtk_menu_shell_append(GTK_MENU_SHELL(parentMenu), menu_item);
    }

    MenuItemNode *new_item = malloc(sizeof(MenuItemNode));
    new_item->menu_id = mii->menu_id;
    new_item->signalHandlerId = signalHandlerId;
    new_item->menu_item = menu_item;
    global_menu_items = g_list_prepend(global_menu_items, new_item);
    it = global_menu_items;
  }
  GtkWidget *menu_item = GTK_WIDGET(((MenuItemNode *)(it->data))->menu_item);
  gtk_widget_set_sensitive(menu_item, mii->disabled != 1);
  gtk_widget_show(menu_item);

  free(mii->title);
  free(mii->tooltip);
  free(mii);
  return FALSE;
}

gboolean do_add_separator(gpointer data) {
  MenuItemInfo *mii = (MenuItemInfo *)data;
  GtkWidget *separator = gtk_separator_menu_item_new();
  gtk_menu_shell_append(GTK_MENU_SHELL(global_tray_menu), separator);
  gtk_widget_show(separator);
  free(mii);
  return FALSE;
}

// runs in main thread, should always return FALSE to prevent gtk to execute it
// again
gboolean do_hide_menu_item(gpointer data) {
  MenuItemInfo *mii = (MenuItemInfo *)data;
  GList *it;
  for (it = global_menu_items; it != NULL; it = it->next) {
    MenuItemNode *item = (MenuItemNode *)(it->data);
    if (item->menu_id == mii->menu_id) {
      gtk_widget_hide(GTK_WIDGET(item->menu_item));
      break;
    }
  }
  free(mii);
  return FALSE;
}

// runs in main thread, should always return FALSE to prevent gtk to execute it
// again
gboolean do_show_menu_item(gpointer data) {
  MenuItemInfo *mii = (MenuItemInfo *)data;
  GList *it;
  for (it = global_menu_items; it != NULL; it = it->next) {
    MenuItemNode *item = (MenuItemNode *)(it->data);
    if (item->menu_id == mii->menu_id) {
      gtk_widget_show(GTK_WIDGET(item->menu_item));
      break;
    }
  }
  free(mii);
  return FALSE;
}

// runs in main thread, should always return FALSE to prevent gtk to execute it
// again
gboolean do_quit(gpointer data) {
  _unlink_temp_file();
  g_list_free_full(global_menu_items, g_free);
  global_menu_items = NULL;
  // app indicator doesn't provide a way to remove it, hide it as a workaround
  if (global_app_indicator != NULL) {
    p_app_indicator_set_status(global_app_indicator,
                               APP_INDICATOR_STATUS_PASSIVE);
  }
  return FALSE;
}

void setIcon(const char *iconBytes, int length, bool template) {
  GBytes *bytes = g_bytes_new_static(iconBytes, length);
  g_idle_add(do_set_icon, bytes);
}

void setTitle(char *ctitle) {
  if (global_app_indicator != NULL) {
    p_app_indicator_set_title(global_app_indicator, ctitle);
    p_app_indicator_set_label(global_app_indicator, ctitle, "");
  }
  free(ctitle);
}

void setMenuItemIcon(const char *iconBytes, int length, int menuId,
                     bool template) {}

void add_or_update_menu_item(int menu_id, int parent_menu_id, char *title,
                             char *tooltip, short disabled, short checked,
                             short isCheckable) {
  MenuItemInfo *mii = malloc(sizeof(MenuItemInfo));
  mii->menu_id = menu_id;
  mii->parent_menu_id = parent_menu_id;
  mii->title = title;
  mii->tooltip = tooltip;
  mii->disabled = disabled;
  mii->checked = checked;
  mii->isCheckable = isCheckable;
  g_idle_add(do_add_or_update_menu_item, mii);
}

void add_separator(int menu_id) {
  MenuItemInfo *mii = malloc(sizeof(MenuItemInfo));
  mii->menu_id = menu_id;
  g_idle_add(do_add_separator, mii);
}

void hide_menu_item(int menu_id) {
  MenuItemInfo *mii = malloc(sizeof(MenuItemInfo));
  mii->menu_id = menu_id;
  g_idle_add(do_hide_menu_item, mii);
}

void show_menu_item(int menu_id) {
  MenuItemInfo *mii = malloc(sizeof(MenuItemInfo));
  mii->menu_id = menu_id;
  g_idle_add(do_show_menu_item, mii);
}

void quit() { g_idle_add(do_quit, NULL); }
