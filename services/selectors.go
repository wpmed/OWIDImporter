package services

const (
	DOWNLOAD_BUTTON_SELECTOR       = `figure div[data-track-note="chart_click_download"] button, .Explorer .ActionButtons div[data-track-note="chart_click_download"] button`
	PLAY_TIMELAPSE_BUTTON_SELECTOR = `.GrapherTimeline`
	DOWNLOAD_SVG_SELECTOR          = "div.download-modal__tab-content:nth-child(1) button.download-button:nth-child(2), div.download-modal__tab-content:nth-child(1) button.download-modal__download-button:nth-child(2)"
	DOWNLOAD_SVG_ICON_SELECTOR     = "div.download-modal__tab-content:nth-child(1) button.download-button:nth-child(2) .download-button__preview-image, div.download-modal__tab-content:nth-child(1) button.download-modal__download-button:nth-child(2) .download-modal__download-preview-img"
	START_MARKER_SELECTOR          = ".startMarker"
	TITLE_SELECTOR                 = "h1.header__title, .HeaderHTML h1"
	END_MARKER_SELECTOR            = ".endMarker"
	DOWNLOAD_POPUP_CLOSE_BUTTON    = "div.download-modal-content button.close-button"
	COUNTRY_SELECTED_OPTIONS_LIST  = ".entity-selector__content .entity-section ul li[data-flip-id^=\"selected_\"], .EntityList label.EntityPickerOption.selected .name"
	COUNTRY_SEARCH_INPUT           = ".entity-selector__search-bar input, .EntityPicker .EntityPickerSearchInput input"
	COUNTRY_SEARCH_RESULT_LIST     = ".entity-selector__content ul li label, .EntityPicker .EntityList label.EntityPickerOption .name"
	MAP_TOOLTIP_SELECTOR           = "#mapTooltip svg"
)
