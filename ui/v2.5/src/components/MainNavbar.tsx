import React, {
  useEffect,
  useRef,
  useState,
  useCallback,
  useMemo,
} from "react";
import {
  defineMessages,
  FormattedMessage,
  MessageDescriptor,
  useIntl,
} from "react-intl";
import { Nav, Navbar, Button } from "react-bootstrap";
import { IconDefinition } from "@fortawesome/fontawesome-svg-core";
import { LinkContainer } from "react-router-bootstrap";
import { Link, NavLink, useLocation, useHistory } from "react-router-dom";
import Mousetrap from "mousetrap";

import SessionUtils from "src/utils/session";
import { Icon } from "src/components/Shared/Icon";
import { useConfigurationContext } from "src/hooks/Config";
import { ManualStateContext } from "./Help/context";
import { SettingsButton } from "./SettingsButton";
import { usePermissions } from "src/hooks/permissions";
import { useToast } from "src/hooks/Toast";
import { queryRandomScene, useConfigureInterface } from "src/core/StashService";
import {
  faBars,
  faChartColumn,
  faDice,
  faFilm,
  faImage,
  faImages,
  faMapMarkerAlt,
  faMoon,
  faPlayCircle,
  faQuestionCircle,
  faSignOutAlt,
  faSun,
  faTag,
  faTimes,
  faUser,
  faVideo,
} from "@fortawesome/free-solid-svg-icons";
import { baseURL } from "src/core/createClient";
import { PatchComponent } from "src/patch";

interface IMenuItem {
  name: string;
  message: MessageDescriptor;
  href: string;
  icon: IconDefinition;
  hotkey: string;
  userCreatable?: boolean;
}
const messages = defineMessages({
  scenes: {
    id: "scenes",
    defaultMessage: "Scenes",
  },
  images: {
    id: "images",
    defaultMessage: "Images",
  },
  groups: {
    id: "groups",
    defaultMessage: "Groups",
  },
  markers: {
    id: "markers",
    defaultMessage: "Markers",
  },
  performers: {
    id: "performers",
    defaultMessage: "Performers",
  },
  studios: {
    id: "studios",
    defaultMessage: "Studios",
  },
  tags: {
    id: "tags",
    defaultMessage: "Tags",
  },
  galleries: {
    id: "galleries",
    defaultMessage: "Galleries",
  },
  sceneTagger: {
    id: "sceneTagger",
    defaultMessage: "Scene Tagger",
  },
  statistics: {
    id: "statistics",
    defaultMessage: "Statistics",
  },
  random: {
    id: "random",
    defaultMessage: "Random",
  },
  random_empty: {
    id: "random_empty",
    defaultMessage: "No scenes available to play randomly.",
  },
  theme_switch_to_light: {
    id: "theme_switch_to_light",
    defaultMessage: "Switch to light theme",
  },
  theme_switch_to_dark: {
    id: "theme_switch_to_dark",
    defaultMessage: "Switch to dark theme",
  },
});

const allMenuItems: IMenuItem[] = [
  {
    name: "scenes",
    message: messages.scenes,
    href: "/scenes",
    icon: faPlayCircle,
    hotkey: "g s",
    userCreatable: true,
  },
  {
    name: "images",
    message: messages.images,
    href: "/images",
    icon: faImage,
    hotkey: "g i",
  },
  {
    name: "groups",
    message: messages.groups,
    href: "/groups",
    icon: faFilm,
    hotkey: "g v",
    userCreatable: true,
  },
  {
    name: "markers",
    message: messages.markers,
    href: "/scenes/markers",
    icon: faMapMarkerAlt,
    hotkey: "g k",
  },
  {
    name: "galleries",
    message: messages.galleries,
    href: "/galleries",
    icon: faImages,
    hotkey: "g l",
    userCreatable: true,
  },
  {
    name: "performers",
    message: messages.performers,
    href: "/performers",
    icon: faUser,
    hotkey: "g p",
    userCreatable: true,
  },
  {
    name: "studios",
    message: messages.studios,
    href: "/studios",
    icon: faVideo,
    hotkey: "g u",
    userCreatable: true,
  },
  {
    name: "tags",
    message: messages.tags,
    href: "/tags",
    icon: faTag,
    hotkey: "g t",
    userCreatable: true,
  },
];

const newPathsList = allMenuItems
  .filter((item) => item.userCreatable)
  .map((item) => item.href);

const MainNavbarMenuItems = PatchComponent(
  "MainNavBar.MenuItems",
  (props: React.PropsWithChildren<unknown>) => {
    return <Nav>{props.children}</Nav>;
  }
);

const MainNavbarUtilityItems = PatchComponent(
  "MainNavBar.UtilityItems",
  (props: React.PropsWithChildren<unknown>) => {
    return <>{props.children}</>;
  }
);

export const MainNavbar: React.FC = () => {
  const history = useHistory();
  const location = useLocation();
  const { configuration } = useConfigurationContext();
  const { openManual } = React.useContext(ManualStateContext);

  const [expanded, setExpanded] = useState(false);

  // Show all menu items by default, unless config says otherwise
  const menuItems = useMemo(() => {
    let cfgMenuItems = configuration?.interface.menuItems;
    if (!cfgMenuItems) {
      return allMenuItems;
    }

    // translate old movies menu item to groups
    cfgMenuItems = cfgMenuItems.map((item) => {
      if (item === "movies") {
        return "groups";
      }
      return item;
    });

    return allMenuItems.filter((menuItem) =>
      cfgMenuItems!.includes(menuItem.name)
    );
  }, [configuration]);

  const showRandom = useMemo(() => {
    const cfgMenuItems = configuration?.interface.menuItems;
    return !cfgMenuItems || cfgMenuItems.includes("random");
  }, [configuration]);

  // react-bootstrap typing bug
  const navbarRef = useRef<HTMLElement | null>(null);
  const intl = useIntl();
  const permissions = usePermissions();
  const Toast = useToast();

  const onRandomClick = useCallback(async () => {
    try {
      const result = await queryRandomScene();
      const scene = result.data?.findScenes.scenes[0];
      if (!scene) {
        Toast.error(intl.formatMessage(messages.random_empty));
        return;
      }
      history.push(`/scenes/${scene.id}`);
    } catch (e) {
      Toast.error(e);
    }
  }, [Toast, history, intl]);

  const isLightTheme = configuration?.interface.theme === "light";
  const [configureInterface] = useConfigureInterface();
  const onThemeToggleClick = useCallback(async () => {
    try {
      await configureInterface({
        variables: { input: { theme: isLightTheme ? "dark" : "light" } },
      });
    } catch (e) {
      Toast.error(e);
    }
  }, [configureInterface, isLightTheme, Toast]);

  const maybeCollapse = useCallback((event: Event) => {
    if (
      navbarRef.current &&
      event.target instanceof Node &&
      !navbarRef.current.contains(event.target)
    ) {
      setExpanded(false);
    }
  }, []);

  useEffect(() => {
    if (expanded) {
      document.addEventListener("click", maybeCollapse);
      document.addEventListener("touchstart", maybeCollapse);
    }
    return () => {
      document.removeEventListener("click", maybeCollapse);
      document.removeEventListener("touchstart", maybeCollapse);
    };
  }, [expanded, maybeCollapse]);

  const goto = useCallback(
    (page: string) => {
      history.push(page);
      if (document.activeElement instanceof HTMLElement) {
        document.activeElement.blur();
      }
    },
    [history]
  );

  const pathname = location.pathname.replace(/\/$/, "");
  let newPath = newPathsList.includes(pathname) ? `${pathname}/new` : null;
  if (newPath !== null) {
    const queryParam = new URLSearchParams(location.search).get("q");
    if (queryParam) {
      newPath += "?q=" + encodeURIComponent(queryParam);
    }
  }

  // set up hotkeys
  useEffect(() => {
    Mousetrap.bind("?", () => openManual());
    Mousetrap.bind("g z", () => goto("/settings"));

    menuItems.forEach((item) => {
      Mousetrap.bind(item.hotkey, () => goto(item.href));
    });

    if (newPath) {
      Mousetrap.bind("n", () => history.push(String(newPath)));
    }

    return () => {
      Mousetrap.unbind("?");
      Mousetrap.unbind("g z");
      menuItems.forEach((item) => {
        Mousetrap.unbind(item.hotkey);
      });

      if (newPath) {
        Mousetrap.unbind("n");
      }
    };
  });

  function maybeRenderLogout() {
    // With the multi-user layer active the sign-out target comes from the
    // server, so an OIDC session ends at the identity provider rather than
    // just dropping stash's own cookie.
    if (permissions.multiUser) {
      return (
        <Button
          className="minimal logout-button d-flex align-items-center"
          href={`${baseURL}${permissions.logoutUrl.replace(/^\//, "")}`}
          title={
            permissions.user
              ? `${intl.formatMessage({ id: "actions.logout" })} (${
                  permissions.user.username
                })`
              : intl.formatMessage({ id: "actions.logout" })
          }
        >
          <Icon icon={faSignOutAlt} />
        </Button>
      );
    }

    if (SessionUtils.isLoggedIn()) {
      return (
        <Button
          className="minimal logout-button d-flex align-items-center"
          href={`${baseURL}logout`}
          title={intl.formatMessage({ id: "actions.logout" })}
        >
          <Icon icon={faSignOutAlt} />
        </Button>
      );
    }
  }

  const handleDismiss = useCallback(() => setExpanded(false), []);

  function renderUtilityButtons() {
    return (
      <>
        {permissions.can("personalise") && (
          <Button
            className="minimal nav-utility theme-toggle"
            title={intl.formatMessage(
              isLightTheme
                ? messages.theme_switch_to_dark
                : messages.theme_switch_to_light
            )}
            onClick={() => {
              handleDismiss();
              onThemeToggleClick();
            }}
          >
            <Icon icon={isLightTheme ? faMoon : faSun} />
          </Button>
        )}
        <NavLink
          className="nav-utility"
          exact
          to="/stats"
          onClick={handleDismiss}
        >
          <Button
            className="minimal d-flex align-items-center h-100"
            title={intl.formatMessage({ id: "statistics" })}
          >
            <Icon icon={faChartColumn} />
          </Button>
        </NavLink>
        <NavLink
          className="nav-utility"
          exact
          to="/settings"
          onClick={handleDismiss}
        >
          <SettingsButton />
        </NavLink>
        <Button
          className="nav-utility minimal"
          onClick={() => openManual()}
          title={intl.formatMessage({ id: "help" })}
        >
          <Icon icon={faQuestionCircle} />
        </Button>
        {maybeRenderLogout()}
      </>
    );
  }

  return (
    <Navbar
      collapseOnSelect
      fixed="top"
      variant="dark"
      bg="dark"
      className="top-nav"
      expand="xl"
      expanded={expanded}
      onToggle={setExpanded}
      ref={navbarRef}
    >
      <Navbar.Collapse className="bg-dark order-sm-1">
        <MainNavbarMenuItems>
          {menuItems.map(({ href, icon, message }) => (
            <Nav.Link
              eventKey={href}
              as="div"
              key={href}
              className="col-4 col-sm-3 col-md-2 col-lg-auto"
            >
              <LinkContainer activeClassName="active" exact to={href}>
                <Button className="minimal p-4 p-xl-2 d-flex d-xl-inline-block flex-column justify-content-between align-items-center">
                  <Icon
                    {...{ icon }}
                    className="nav-menu-icon d-block d-xl-inline mb-2 mb-xl-0"
                  />
                  <span>{intl.formatMessage(message)}</span>
                </Button>
              </LinkContainer>
            </Nav.Link>
          ))}
          {permissions.can("view_library") && showRandom && (
            <Nav.Link
              as="div"
              className="col-4 col-sm-3 col-md-2 col-lg-auto"
            >
              <Button
                className="minimal random-scene p-4 p-xl-2 d-flex d-xl-inline-block flex-column justify-content-between align-items-center"
                title={intl.formatMessage(messages.random)}
                onClick={() => {
                  handleDismiss();
                  onRandomClick();
                }}
              >
                <Icon
                  icon={faDice}
                  className="nav-menu-icon d-block d-xl-inline mb-2 mb-xl-0"
                />
                <span>{intl.formatMessage(messages.random)}</span>
              </Button>
            </Nav.Link>
          )}
        </MainNavbarMenuItems>
        <Nav>
          <MainNavbarUtilityItems>
            {renderUtilityButtons()}
          </MainNavbarUtilityItems>
        </Nav>
      </Navbar.Collapse>

      <Navbar.Brand as="div" onClick={handleDismiss}>
        <Link to="/">
          <Button className="minimal brand-link d-inline-block">Stash</Button>
        </Link>
      </Navbar.Brand>

      <Nav className="navbar-buttons flex-row ml-auto order-xl-2">
        {!!newPath && (
          <div className="mr-2">
            <Link to={newPath}>
              <Button variant="primary" data-action="new">
                <FormattedMessage id="new" defaultMessage="New" />
              </Button>
            </Link>
          </div>
        )}
        <MainNavbarUtilityItems>
          {renderUtilityButtons()}
        </MainNavbarUtilityItems>
        <Navbar.Toggle className="nav-menu-toggle ml-sm-2">
          <Icon icon={expanded ? faTimes : faBars} />
        </Navbar.Toggle>
      </Nav>
    </Navbar>
  );
};
