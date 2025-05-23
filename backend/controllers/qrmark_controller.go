package controllers

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/mux"

	"github.com/v420v/qrmarkapi/apierrors"
	"github.com/v420v/qrmarkapi/common"
	"github.com/v420v/qrmarkapi/controllers/services"
	"github.com/v420v/qrmarkapi/models"
)

type QrmarkController struct {
	service services.QrmarkServicer
}

func NewQrmarkController(service services.QrmarkServicer) *QrmarkController {
	return &QrmarkController{service: service}
}

func (c *QrmarkController) GetUserTotalPointsHandler(w http.ResponseWriter, req *http.Request) {
	userID, err := strconv.Atoi(mux.Vars(req)["id"])
	if err != nil {
		apierrors.ErrorHandler(w, req, err)
		return
	}

	userTotalPoints, err := c.service.SelectUserTotalPointsService(userID)
	if err != nil {
		apierrors.ErrorHandler(w, req, err)
		return
	}

	json.NewEncoder(w).Encode(userTotalPoints)
}

func (c *QrmarkController) GetSchoolPointsHandler(w http.ResponseWriter, req *http.Request) {
	schoolID, err := strconv.Atoi(mux.Vars(req)["id"])
	if err != nil {
		apierrors.ErrorHandler(w, req, err)
		return
	}

	schoolPoints, err := c.service.SelectSchoolPointsService(schoolID)
	if err != nil {
		apierrors.ErrorHandler(w, req, err)
		return
	}

	json.NewEncoder(w).Encode(schoolPoints)
}

func loadECDSAPublicKey() (*ecdsa.PublicKey, error) {
	keyBytes, err := os.ReadFile("/home/ec2-user/QRmark/keys/ecdsa_p256_public_key.pem")
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(keyBytes)
	if block == nil {
		return nil, errors.New("failed to decode PEM block")
	}

	if block.Type != "PUBLIC KEY" {
		return nil, errors.New("not a valid public key")
	}

	pubKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("error parsing public key: %w", err)
	}

	ecdsaPubKey, ok := pubKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, errors.New("the public key is not an ECDSA key")
	}

	return ecdsaPubKey, nil
}

func verifyJWTWithECDSASignature(publicKey *ecdsa.PublicKey, tokenString string) (*jwt.Token, error) {
	token, err := jwt.Parse(
		tokenString,
		func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodECDSA); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Method)
			}
			return publicKey, nil
		},
	)

	if err != nil {
		return nil, err
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	return token, nil
}

func (c *QrmarkController) GetQrmarkListHandler(w http.ResponseWriter, req *http.Request) {
	var page int = 0

	queryMap := req.URL.Query()

	if p, ok := queryMap["page"]; ok && len(p) > 0 {
		var err error
		page, err = strconv.Atoi(p[0])
		if err != nil {
			err = apierrors.BadParam.Wrap(err, "page in query param must be number")
			apierrors.ErrorHandler(w, req, err)
			return
		}
	} else {
		page = 1
	}

	var err error = nil
	var qrmarkList models.QrmarkList

	if p, ok := queryMap["user"]; ok && len(p) > 0 {
		var err error
		userID, err := strconv.Atoi(p[0])
		if err != nil {
			err = apierrors.BadParam.Wrap(err, "page in query param must be number")
			apierrors.ErrorHandler(w, req, err)
			return
		}
		qrmarkList, err = c.service.SelectUserQrmarkListService(userID, page)
	} else {
		qrmarkList, err = c.service.SelectQrmarkListService(page)
	}

	if err != nil {
		apierrors.ErrorHandler(w, req, err)
		return
	}

	json.NewEncoder(w).Encode(qrmarkList)
}

func (c *QrmarkController) PostQrmarkHandler(w http.ResponseWriter, req *http.Request) {
	type ReqData struct {
		Jwt string `json:"jwt"`
	}

	var reqData ReqData

	if err := json.NewDecoder(req.Body).Decode(&reqData); err != nil {
		apierrors.ErrorHandler(w, req, err)
		return
	}

	publicKey, err := loadECDSAPublicKey()
	if err != nil {
		apierrors.ErrorHandler(w, req, err)
		return
	}
	token, err := verifyJWTWithECDSASignature(publicKey, reqData.Jwt)
	if err != nil {
		apierrors.ErrorHandler(w, req, err)
		return
	}

	claims := token.Claims.(jwt.MapClaims)

	qrmarkID, subOk := claims["sub"].(float64)
	qrmarkNumber, numOk := claims["num"].(float64)
	qrmarkPoint, pointOk := claims["point"].(float64)

	if !subOk || !numOk || !pointOk {
		apierrors.ErrorHandler(w, req, errors.New("invalid jwt claims"))
		return
	}

	userID, err := common.GetCurrentUserID(req.Context())
	if err != nil {
		apierrors.ErrorHandler(w, req, err)
		return
	}

	qrmarkInfo := models.QrmarkInfo{
		QrmarkID:  int(qrmarkID),
		UserID:    userID,
		CompanyID: int(qrmarkNumber),
		Point:     int(qrmarkPoint),
	}

	err = c.service.InsertQrmarkService(qrmarkInfo)
	if err != nil {
		apierrors.ErrorHandler(w, req, err)
		return
	}
}
