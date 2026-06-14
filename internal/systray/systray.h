#ifndef SYSTRAY_H
#define SYSTRAY_H
/*
        this file contains code from the systray project
   (https://github.com/getlantern/systray), licensed under the apache license.
        see more in the copying.md file in the root directory of this project.
*/

#include "stdbool.h"

extern void systray_ready();
extern void systray_on_exit();
extern void systray_menu_item_selected(int menu_id);
void registerSystray(void);
int nativeLoop(void);

void setIcon(const char* iconBytes, int length, bool isTemplate);
void setMenuItemIcon(const char* iconBytes, int length, int menuId, bool isTemplate);
void setTitle(char* title);
void setTooltip(char* tooltip);
void setRemovalAllowed(bool allowed);
void add_or_update_menu_item(int menuId, int parentMenuId, char* title, char* tooltip, short disabled, short checked, short isCheckable);
void add_separator(int menuId);
void hide_menu_item(int menuId);
void show_menu_item(int menuId);
void quit();

#endif //SYSTRAY_H
