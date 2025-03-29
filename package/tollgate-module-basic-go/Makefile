include $(TOPDIR)/rules.mk

PKG_NAME:=tollgate-module-basic-go
PKG_VERSION:=$(shell git rev-list --count HEAD 2>/dev/null || echo "0.0.1").$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
PKG_RELEASE:=1

# Place conditional checks EARLY - before variables that depend on them
ifneq ($(TOPDIR),)
	# Feed-specific settings (auto-clone from git)
	PKG_SOURCE_PROTO:=git
	PKG_SOURCE_URL:=https://github.com/OpenTollGate/tollgate-module-basic-go.git
	PKG_SOURCE_VERSION:=$(shell git rev-parse HEAD) # Use exact current commit
	PKG_MIRROR_HASH:=skip
else
	# SDK build context (local files)
	PKG_BUILD_DIR:=$(CURDIR)
endif

PKG_MAINTAINER:=Your Name <your@email.com>
PKG_LICENSE:=CC0-1.0
PKG_LICENSE_FILES:=LICENSE

PKG_BUILD_DEPENDS:=golang/host
PKG_BUILD_PARALLEL:=1
PKG_USE_MIPS16:=0

GO_PKG:=github.com/OpenTollGate/tollgate-module-basic-go

include $(INCLUDE_DIR)/package.mk
$(eval $(call GoPackage))

define Package/$(PKG_NAME)
	SECTION:=net
	CATEGORY:=Network
	TITLE:=TollGate Basic Module
	DEPENDS:=$(GO_ARCH_DEPENDS) +nodogsplash
endef

define Package/$(PKG_NAME)/description
	TollGate Basic Module for OpenWrt
endef

define Build/Prepare
	$(call Build/Prepare/Default)
	echo "DEBUG: Contents of go.mod after prepare:"
	cat $(PKG_BUILD_DIR)/go.mod
endef

define Build/Configure
endef

define Build/Compile
	cd $(PKG_BUILD_DIR) && \
	GOOS=linux \
	GOARCH=$(if $(CONFIG_ARCH_64BIT),arm64,$(if $(CONFIG_TARGET_ath79),mips,arm)) \
	$(if $(CONFIG_TARGET_ath79),GOMIPS=softfloat,) \
	go build -o $(PKG_NAME) -trimpath -ldflags="-s -w" 
endef

define Package/$(PKG_NAME)/install
	$(INSTALL_DIR) $(1)/usr/bin
	$(INSTALL_BIN) $(PKG_BUILD_DIR)/$(PKG_NAME) $(1)/usr/bin/tollgate-basic
	
	# Init script
	$(INSTALL_DIR) $(1)/etc/init.d
	$(INSTALL_BIN) ./files/etc/init.d/tollgate-basic $(1)/etc/init.d/
	
	# UCI defaults for configuration
	$(INSTALL_DIR) $(1)/etc/uci-defaults
	$(INSTALL_BIN) ./files/etc/uci-defaults/99-tollgate-setup $(1)/etc/uci-defaults/

	# UCI defaults for random LAN IP
	$(INSTALL_DIR) $(1)/etc/uci-defaults
	$(INSTALL_BIN) ./files/etc/uci-defaults/95-random-lan-ip $(1)/etc/uci-defaults/
	
	# Keep only TollGate-specific configs
	$(INSTALL_DIR) $(1)/etc/config
	$(INSTALL_DATA) ./files/etc/config/firewall-tollgate $(1)/etc/config/
	
	# System files (non-UCI)
	$(INSTALL_DIR) $(1)/etc/modt
	$(INSTALL_DATA) ./files/etc/modt/* $(1)/etc/modt/
	
	# First-login setup
	$(INSTALL_DIR) $(1)/usr/local/bin
	$(INSTALL_BIN) ./files/usr/local/bin/first-login-setup $(1)/usr/local/bin/
	# Create postinst script to display LuCI URL message
	$(INSTALL_DIR) $(1)/CONTROL
	echo "#!/bin/sh" > $(1)/CONTROL/postinst
	echo "echo ''" >> $(1)/CONTROL/postinst
	echo "echo '╔════════════════════════════════════════════════════════════════╗'" >> $(1)/CONTROL/postinst
	echo "echo '║ TollGate Module installation complete!                         ║'" >> $(1)/CONTROL/postinst
	echo "echo '║ Access the LuCI web interface at:                              ║'" >> $(1)/CONTROL/postinst
	echo "echo '║ http://'\`uci get network.lan.ipaddr\`':8080                        ║'" >> $(1)/CONTROL/postinst
	echo "echo '║ Use this interface to configure your TollGate settings.        ║'" >> $(1)/CONTROL/postinst
	echo "echo '╚════════════════════════════════════════════════════════════════╝'" >> $(1)/CONTROL/postinst
	echo "echo ''" >> $(1)/CONTROL/postinst
	echo "exit 0" >> $(1)/CONTROL/postinst
	chmod 755 $(1)/CONTROL/postinst
endef

# Update FILES declaration
FILES_$(PKG_NAME) += \
	/usr/bin/tollgate-basic \
	/etc/init.d/tollgate-basic \
	/etc/config/firewall-tollgate \
	/etc/modt/* \
	/etc/profile \
	/usr/local/bin/first-login-setup \
	/etc/uci-defaults/99-tollgate-setup \
	/etc/uci-defaults/95-random-lan-ip

$(eval $(call BuildPackage,$(PKG_NAME)))

# Print IPK path after successful compilation
PKG_FINISH:=$(shell echo "Successfully built: $(IPK_FILE)" >&2)
